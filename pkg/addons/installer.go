package addons

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/monshunter/ohmykube/pkg/cache"
	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
)

// UniversalInstaller is a generic installer for any addon type
type UniversalInstaller struct {
	sshRunner      interfaces.SSHRunner
	controllerNode string
	spec           config.AddonSpec
	cluster        *config.Cluster
}

// NewUniversalInstaller creates a new universal installer instance
func NewUniversalInstaller(sshRunner interfaces.SSHRunner, controllerNode string, spec config.AddonSpec, cluster *config.Cluster) *UniversalInstaller {
	return &UniversalInstaller{
		sshRunner:      sshRunner,
		controllerNode: controllerNode,
		spec:           spec,
		cluster:        cluster,
	}
}

// Install determines and executes the required operation based on current state
// This method is kept for backward compatibility. New code should use the
// task-based approach with ExecuteOperation() for better control and testing.
func (u *UniversalInstaller) Install() error {
	// Get current status and determine operation using cluster's centralized logic
	currentStatus, exists := u.cluster.GetAddonStatus(u.spec.Name)
	operation, reason := u.cluster.DetermineAddonOperation(u.spec, currentStatus, exists)

	return u.ExecuteOperation(operation, reason)
}

// ExecuteOperation executes a specific operation that has been pre-determined
// This is the preferred method when using the task-based addon management system.
func (u *UniversalInstaller) ExecuteOperation(operation config.AddonOperation, reason string) error {
	switch operation {
	case config.AddonOperationInstall:
		return u.performInstall(reason)
	case config.AddonOperationUpdate:
		return u.performUpdate(reason)
	case config.AddonOperationRemove:
		return u.performRemove(reason)
	case config.AddonOperationNoChange:
		log.Infof("✅ Addon %s is up-to-date: %s", u.spec.Name, reason)
		return nil
	default:
		return fmt.Errorf("unknown operation: %s", operation)
	}
}

// performInstall performs a fresh installation
func (u *UniversalInstaller) performInstall(reason string) error {
	log.Debugf("Installing addon: %s (type: %s, version: %s) - %s", u.spec.Name, u.spec.Type, u.spec.Version, reason)

	// 1. Set initial status
	u.setAddonPhase(config.AddonPhaseInstalling, "Starting installation", "Installing")

	// 2. Execute pre-install hooks
	if err := u.executeHooks(u.spec.PreInstall, "pre-install"); err != nil {
		u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Pre-install hook failed: %v", err), "PreInstallFailed")
		return fmt.Errorf("pre-install hook failed: %w", err)
	}

	// 3. Cache images (before file preparation to use original local paths)
	u.setAddonPhase(config.AddonPhaseInstalling, "Caching images", "CachingImages")
	if err := u.cacheImages(); err != nil {
		log.Warningf("Failed to cache images for %s: %v", u.spec.Name, err)
	}

	// 4. Prepare files (upload local files or download remote files) - after image caching
	if (len(u.spec.ValuesFiles) > 0 && u.spec.Type == "helm") || (len(u.spec.Files) > 0 && u.spec.Type == "manifest") || u.isLocalChart() {
		u.setAddonPhase(config.AddonPhaseInstalling, "Preparing files", "PreparingFiles")
		if err := u.prepareFiles(); err != nil {
			u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Failed to prepare files: %v", err), "PrepareFilesFailed")
			return fmt.Errorf("failed to prepare files: %w", err)
		}
	}

	// 5. Execute installation
	u.setAddonPhase(config.AddonPhaseInstalling, "Installing application", "Installing")

	var err error
	switch u.spec.Type {
	case "helm":
		err = u.installHelm()
	case "manifest":
		err = u.installManifest()
	default:
		err = fmt.Errorf("unsupported addon type: %s", u.spec.Type)
	}

	if err != nil {
		u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Installation failed: %v", err), "InstallationFailed")
		return err
	}

	// 5. Execute post-install hooks
	if err := u.executeHooks(u.spec.PostInstall, "post-install"); err != nil {
		u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Post-install hook failed: %v", err), "PostInstallFailed")
		return fmt.Errorf("post-install hook failed: %w", err)
	}

	// 6. Verify installation
	u.setAddonPhase(config.AddonPhaseInstalling, "Verifying installation", "Verifying")
	if err := u.verifyInstallation(); err != nil {
		u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Verification failed: %v", err), "VerificationFailed")
		return fmt.Errorf("verification failed: %w", err)
	}

	// 7. Set final status
	u.setFinalStatus()

	log.Infof("✅ Addon %s installed successfully", u.spec.Name)
	return nil
}

// performUpdate performs an update of an existing installation
func (u *UniversalInstaller) performUpdate(reason string) error {
	log.Infof("Updating addon: %s (type: %s, version: %s) - %s", u.spec.Name, u.spec.Type, u.spec.Version, reason)

	// Set updating status
	u.setAddonPhase(config.AddonPhaseUpgrading, "Starting update", "Updating")

	// For most cases, update is similar to install but with different Helm command
	var err error
	switch u.spec.Type {
	case "helm":
		err = u.upgradeHelm()
	case "manifest":
		// For manifests, we'll reinstall
		err = u.installManifest()
	default:
		err = fmt.Errorf("unsupported addon type: %s", u.spec.Type)
	}

	if err != nil {
		u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Update failed: %v", err), "UpdateFailed")
		return err
	}

	// Execute post-install hooks (treating update as install for hooks)
	if err := u.executeHooks(u.spec.PostInstall, "post-update"); err != nil {
		u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Post-update hook failed: %v", err), "PostUpdateFailed")
		return fmt.Errorf("post-update hook failed: %w", err)
	}

	// Verify installation
	u.setAddonPhase(config.AddonPhaseUpgrading, "Verifying update", "Verifying")
	if err := u.verifyInstallation(); err != nil {
		u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Update verification failed: %v", err), "UpdateVerificationFailed")
		return fmt.Errorf("update verification failed: %w", err)
	}

	// Set final status
	u.setFinalStatus()

	log.Infof("✅ Addon %s updated successfully", u.spec.Name)
	return nil
}

// performRemove removes an existing installation
func (u *UniversalInstaller) performRemove(reason string) error {
	log.Infof("Removing addon: %s - %s", u.spec.Name, reason)

	// Set removing status
	u.setAddonPhase(config.AddonPhaseRemoving, "Starting removal", "Removing")

	// Remove based on type
	var err error
	switch u.spec.Type {
	case "helm":
		err = u.removeHelm()
	case "manifest":
		err = u.removeManifest()
	default:
		err = fmt.Errorf("unsupported addon type for removal: %s", u.spec.Type)
	}

	if err != nil {
		u.setAddonPhase(config.AddonPhaseFailed, fmt.Sprintf("Removal failed: %v", err), "RemovalFailed")
		return err
	}

	// Set final removed status
	u.setAddonPhase(config.AddonPhaseRemoved, "Addon removed successfully", "Removed")

	log.Infof("✅ Addon %s removed successfully", u.spec.Name)
	return nil
}

// installHelm installs a Helm chart
func (u *UniversalInstaller) installHelm() error {
	// Handle remote charts (add repository)
	if !u.isLocalChart() {
		// 1. Add Helm repository
		repoName := u.getRepoName()
		addRepoCmd := fmt.Sprintf(`helm repo add %s %s || true && helm repo update`, repoName, u.spec.Repo)

		_, err := u.sshRunner.RunCommand(u.controllerNode, addRepoCmd)
		if err != nil {
			return fmt.Errorf("failed to add helm repository: %w", err)
		}
	}

	// 2. Build helm install command
	installCmd := u.buildHelmInstallCommand()

	_, err := u.sshRunner.RunCommand(u.controllerNode, installCmd)
	if err != nil {
		return fmt.Errorf("failed to install helm chart: %w", err)
	}

	return nil
}

// buildHelmInstallCommand builds the helm install command
func (u *UniversalInstaller) buildHelmInstallCommand() string {
	chartName := u.spec.Chart

	// Handle local vs remote charts
	if u.isLocalChart() {
		// For local charts, use the chart path directly (already uploaded to remote)
		cmd := fmt.Sprintf("helm install %s %s", u.spec.Name, chartName)
		
		// Only add version for remote charts
		// For local charts, version is controlled by Chart.yaml in the chart directory
		
		// Add namespace
		if u.spec.Namespace != "" {
			cmd += fmt.Sprintf(" --namespace %s --create-namespace", u.spec.Namespace)
		}
		
		// Add values files
		for _, valuesFile := range u.spec.ValuesFiles {
			cmd += fmt.Sprintf(" --values %s", valuesFile)
		}

		// Add values
		for key, value := range u.spec.Values {
			cmd += fmt.Sprintf(" --set %s=%s", key, value)
		}

		// Add timeout
		timeout := u.spec.Timeout
		if timeout == "" {
			timeout = "300s"
		}
		cmd += fmt.Sprintf(" --wait --timeout %s", timeout)
		
		return cmd
	} else {
		// Handle remote charts
		repoName := u.getRepoName()

		// Use repo name prefix if chart doesn't already include it
		if !strings.Contains(chartName, "/") {
			chartName = fmt.Sprintf("%s/%s", repoName, chartName)
		}

		cmd := fmt.Sprintf("helm install %s %s --version %s", u.spec.Name, chartName, u.spec.Version)
		
		// Add namespace
		if u.spec.Namespace != "" {
			cmd += fmt.Sprintf(" --namespace %s --create-namespace", u.spec.Namespace)
		}
		// Add values files
		for _, valuesFile := range u.spec.ValuesFiles {
			cmd += fmt.Sprintf(" --values %s", valuesFile)
		}

		// Add values
		for key, value := range u.spec.Values {
			cmd += fmt.Sprintf(" --set %s=%s", key, value)
		}

		// Add timeout
		timeout := u.spec.Timeout
		if timeout == "" {
			timeout = "300s"
		}
		cmd += fmt.Sprintf(" --wait --timeout %s", timeout)
		
		return cmd
	}
}

// getRepoName extracts or generates a repository name
func (u *UniversalInstaller) getRepoName() string {
	// Try to extract repo name from URL
	parts := strings.Split(u.spec.Repo, "/")
	if len(parts) >= 2 {
		// Use the last part of the path as repo name
		repoName := strings.TrimSuffix(parts[len(parts)-1], ".git")
		if repoName != "" {
			return repoName
		}
	}

	// Fallback to addon name
	return fmt.Sprintf("%s-repo", u.spec.Name)
}

// installManifest installs Kubernetes manifests
func (u *UniversalInstaller) installManifest() error {
	if len(u.spec.Files) == 0 {
		return fmt.Errorf("no files specified for manifest addon")
	}

	installCmd := fmt.Sprintf("kubectl apply -f %s", strings.Join(u.spec.Files, " -f "))

	// Add namespace
	if u.spec.Namespace != "" {
		installCmd += fmt.Sprintf(" --namespace %s", u.spec.Namespace)
	}

	_, err := u.sshRunner.RunCommand(u.controllerNode, installCmd)
	if err != nil {
		return fmt.Errorf("failed to apply manifest: %w", err)
	}
	return nil
}

// setAddonPhase sets the addon phase and saves cluster state
func (u *UniversalInstaller) setAddonPhase(phase config.AddonPhase, message, reason string) {
	u.cluster.SetAddonPhase(u.spec.Name, phase, message, reason)

	// Save cluster state
	if err := u.cluster.Save(); err != nil {
		log.Warningf("Failed to save cluster state: %v", err)
	}
}

// executeHooks executes a list of command hooks
func (u *UniversalInstaller) executeHooks(hooks []string, hookType string) error {
	for i, hook := range hooks {
		log.Debugf("Executing %s hook %d/%d for %s: %s", hookType, i+1, len(hooks), u.spec.Name, hook)

		_, err := u.sshRunner.RunCommand(u.controllerNode, hook)
		if err != nil {
			return fmt.Errorf("hook %d failed: %w", i+1, err)
		}
	}
	return nil
}

// verifyInstallation verifies the installation based on addon type
func (u *UniversalInstaller) verifyInstallation() error {
	switch u.spec.Type {
	case "helm":
		return u.verifyHelmInstallation()
	case "manifest":
		return u.verifyManifestInstallation()
	default:
		return fmt.Errorf("unsupported addon type: %s", u.spec.Type)
	}
}

// verifyHelmInstallation verifies Helm installation
func (u *UniversalInstaller) verifyHelmInstallation() error {
	// Check if Helm release exists and is deployed
	checkCmd := fmt.Sprintf("helm status %s", u.spec.Name)
	if u.spec.Namespace != "" {
		checkCmd += fmt.Sprintf(" --namespace %s", u.spec.Namespace)
	}

	output, err := u.sshRunner.RunCommand(u.controllerNode, checkCmd)
	if err != nil {
		return fmt.Errorf("helm release verification failed: %w", err)
	}

	// Check if status contains "deployed"
	if !strings.Contains(strings.ToLower(output), "deployed") {
		return fmt.Errorf("helm release is not in deployed state: %s", output)
	}

	return nil
}

// verifyManifestInstallation verifies manifest installation
func (u *UniversalInstaller) verifyManifestInstallation() error {
	// For manifest installations, we'll do a basic check to see if resources exist
	// This is a simple verification - in production, you might want more sophisticated checks

	checkCmd := "kubectl get all"
	if u.spec.Namespace != "" {
		checkCmd += fmt.Sprintf(" --namespace %s", u.spec.Namespace)
	}

	_, err := u.sshRunner.RunCommand(u.controllerNode, checkCmd)
	if err != nil {
		return fmt.Errorf("manifest verification failed: %w", err)
	}

	return nil
}

// setFinalStatus sets the final success status
func (u *UniversalInstaller) setFinalStatus() {
	now := time.Now()
	status := config.AddonStatus{
		Name:             u.spec.Name,
		Phase:            config.AddonPhaseInstalled,
		InstalledVersion: u.spec.Version,
		DesiredVersion:   u.spec.Version,
		Namespace:        u.spec.Namespace,
		InstallTime:      &now,
		LastUpdateTime:   &now,
		Message:          "Installation completed successfully",
		Reason:           "Installed",
	}

	u.cluster.SetAddonStatus(u.spec.Name, status)
}

// cacheImages caches images following the existing plugin pattern
func (u *UniversalInstaller) cacheImages() error {
	ctx := context.Background()

	// Get existing ImageManager
	imageManager, err := cache.GetImageManager()
	if err != nil {
		return fmt.Errorf("failed to get image manager: %w", err)
	}

	// Build ImageSource based on addon type
	var source interfaces.ImageSource
	switch u.spec.Type {
	case "helm":
		source = interfaces.ImageSource{
			Type:         "helm",
			ChartName:    u.spec.Chart,
			ChartRepo:    u.spec.Repo,
			Version:      u.spec.Version,
			ChartValues:  u.spec.Values,
			ValuesFile:   u.spec.ValuesFiles,
			IsLocalChart: u.isLocalChart(),
		}
	case "manifest":
		source = interfaces.ImageSource{
			Type:          "manifest",
			ManifestFiles: u.spec.Files,
			Version:       u.spec.Version,
		}
	}

	// Call existing image caching method
	err = imageManager.EnsureImages(ctx, source, u.sshRunner, u.controllerNode, u.controllerNode)

	if err != nil {
		return fmt.Errorf("failed to cache Flannel images: %w", err)
	}
	_ = imageManager.ReCacheClusterImages(u.sshRunner)
	return nil
}

// upgradeHelm upgrades an existing Helm chart
func (u *UniversalInstaller) upgradeHelm() error {
	// 1. Update Helm repository
	repoName := u.getRepoName()
	updateRepoCmd := fmt.Sprintf(`helm repo update %s`, repoName)

	_, err := u.sshRunner.RunCommand(u.controllerNode, updateRepoCmd)
	if err != nil {
		log.Warningf("Failed to update helm repository: %v", err)
	}

	// 2. Build helm upgrade command
	upgradeCmd := u.buildHelmUpgradeCommand()

	_, err = u.sshRunner.RunCommand(u.controllerNode, upgradeCmd)
	if err != nil {
		return fmt.Errorf("failed to upgrade helm chart: %w", err)
	}

	return nil
}

// removeHelm removes a Helm chart
func (u *UniversalInstaller) removeHelm() error {
	namespace := u.spec.Namespace
	if namespace == "" {
		namespace = "default"
	}

	uninstallCmd := fmt.Sprintf(`helm uninstall %s -n %s`, u.spec.Name, namespace)

	_, err := u.sshRunner.RunCommand(u.controllerNode, uninstallCmd)
	if err != nil {
		return fmt.Errorf("failed to uninstall helm chart: %w", err)
	}

	return nil
}

// removeManifest removes resources from manifest
func (u *UniversalInstaller) removeManifest() error {
	// Delete all manifest files
	for _, file := range u.spec.Files {
		deleteCmd := fmt.Sprintf(`kubectl delete -f %s --ignore-not-found=true`, file)
		_, err := u.sshRunner.RunCommand(u.controllerNode, deleteCmd)
		if err != nil {
			return fmt.Errorf("failed to delete manifest from file %s: %w", file, err)
		}
	}

	return nil
}

// buildHelmUpgradeCommand builds the helm upgrade command
func (u *UniversalInstaller) buildHelmUpgradeCommand() string {
	repoName := u.getRepoName()
	chartName := u.spec.Chart

	// Use repo name prefix if chart doesn't already include it
	if !strings.Contains(chartName, "/") {
		chartName = fmt.Sprintf("%s/%s", repoName, chartName)
	}

	namespace := u.spec.Namespace
	if namespace == "" {
		namespace = "default"
	}

	cmd := fmt.Sprintf(`helm upgrade %s %s --version %s -n %s --create-namespace`,
		u.spec.Name, chartName, u.spec.Version, namespace)

	// Add timeout
	if u.spec.Timeout != "" {
		cmd += fmt.Sprintf(` --timeout %s`, u.spec.Timeout)
	} else {
		cmd += ` --timeout 300s`
	}

	// Add values
	for key, value := range u.spec.Values {
		cmd += fmt.Sprintf(` --set %s=%s`, key, value)
	}

	// Add values files
	for _, valuesFile := range u.spec.ValuesFiles {
		cmd += fmt.Sprintf(` -f %s`, valuesFile)
	}

	return cmd
}

// prepareFiles prepares files (values files for helm, manifest files for manifest) using the shared file manager
func (u *UniversalInstaller) prepareFiles() error {
	ctx := context.Background()
	fileManager := cache.NewDefaultValuesFileManager()

	// Prepare values files for helm addons
	if u.spec.Type == "helm" && len(u.spec.ValuesFiles) > 0 {
		remoteValuesFiles, err := fileManager.PrepareValuesFiles(ctx, u.spec.ValuesFiles, u.sshRunner, u.controllerNode, fmt.Sprintf("addon-%s-values", u.spec.Name))
		if err != nil {
			return fmt.Errorf("failed to prepare values files: %w", err)
		}
		u.spec.ValuesFiles = remoteValuesFiles
		log.Debugf("Prepared %d values files for helm addon %s", len(remoteValuesFiles), u.spec.Name)
	}

	// Prepare local helm charts
	if u.spec.Type == "helm" && u.isLocalChart() {
		remoteChartPath, err := u.prepareLocalChart(ctx, fileManager)
		if err != nil {
			return fmt.Errorf("failed to prepare local chart: %w", err)
		}
		// Update chart path to remote path
		u.spec.Chart = remoteChartPath
		log.Debugf("Prepared local chart for helm addon %s: %s", u.spec.Name, remoteChartPath)
	}

	// Prepare manifest files for manifest addons
	if u.spec.Type == "manifest" && len(u.spec.Files) > 0 {
		remoteManifestFiles, err := fileManager.PrepareValuesFiles(ctx, u.spec.Files, u.sshRunner, u.controllerNode, fmt.Sprintf("addon-%s-manifest", u.spec.Name))
		if err != nil {
			return fmt.Errorf("failed to prepare manifest files: %w", err)
		}
		u.spec.Files = remoteManifestFiles
		log.Debugf("Prepared %d manifest files for manifest addon %s", len(remoteManifestFiles), u.spec.Name)
	}

	return nil
}

// isLocalChart determines if the current spec uses a local chart
func (u *UniversalInstaller) isLocalChart() bool {
	return u.spec.IsLocalChart()
}

// prepareLocalChart handles uploading local charts to the controller node
func (u *UniversalInstaller) prepareLocalChart(ctx context.Context, fileManager interfaces.ValuesFileManager) (string, error) {
	localChartPath := u.spec.Chart
	
	// Generate remote chart path
	remoteChartPath := fmt.Sprintf("/tmp/addon-%s-chart", u.spec.Name)
	
	// Check if it's a directory or file
	if info, err := os.Stat(localChartPath); err == nil {
		if info.IsDir() {
			// Upload directory
			log.Debugf("Uploading local chart directory from %s to %s on controller node", localChartPath, remoteChartPath)
			err := u.sshRunner.UploadDirectory(u.controllerNode, localChartPath, remoteChartPath)
			if err != nil {
				return "", fmt.Errorf("failed to upload chart directory %s: %w", localChartPath, err)
			}
			return remoteChartPath, nil
		} else {
			// Upload single file (chart package)
			remoteChartFile := fmt.Sprintf("/tmp/addon-%s-chart.tgz", u.spec.Name)
			log.Debugf("Uploading local chart package from %s to %s on controller node", localChartPath, remoteChartFile)
			err := u.sshRunner.UploadFile(u.controllerNode, localChartPath, remoteChartFile)
			if err != nil {
				return "", fmt.Errorf("failed to upload chart package %s: %w", localChartPath, err)
			}
			return remoteChartFile, nil
		}
	} else {
		return "", fmt.Errorf("local chart path does not exist: %s", localChartPath)
	}
}
