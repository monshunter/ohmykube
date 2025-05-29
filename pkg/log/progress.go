package log

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ProgressBar represents a progress bar for long-running operations
type ProgressBar struct {
	title      string
	total      int
	current    int
	width      int
	startTime  time.Time
	lastUpdate time.Time
	completed  bool
	showTime   bool
}

// NewProgressBar creates a new progress bar
func NewProgressBar(title string, total int) *ProgressBar {
	return &ProgressBar{
		title:     title,
		total:     total,
		width:     20,
		startTime: time.Now(),
		showTime:  true,
	}
}

// Update updates the progress bar
func (pb *ProgressBar) Update(current int) {
	if quiet {
		return // Don't show progress in quiet mode
	}

	pb.current = current
	pb.lastUpdate = time.Now()
	pb.render()
}

// Increment increments the progress by 1
func (pb *ProgressBar) Increment() {
	pb.Update(pb.current + 1)
}

// Complete marks the progress as completed
func (pb *ProgressBar) Complete() {
	pb.current = pb.total
	pb.completed = true
	pb.render()
	fmt.Println() // New line after completion
}

// render renders the progress bar
func (pb *ProgressBar) render() {
	if quiet {
		return
	}

	percentage := float64(pb.current) / float64(pb.total) * 100
	filled := int(float64(pb.width) * float64(pb.current) / float64(pb.total))

	bar := strings.Repeat("█", filled) + strings.Repeat("░", pb.width-filled)

	timeStr := ""
	if pb.showTime && pb.completed {
		elapsed := time.Since(pb.startTime)
		timeStr = fmt.Sprintf(" (%s)", formatDuration(elapsed))
	}

	status := "100%"
	if !pb.completed {
		status = fmt.Sprintf("%.0f%%", percentage)
	}

	progressMsg := fmt.Sprintf("%s [%s] %s%s", pb.title, bar, status, timeStr)
	if pb.completed {
		progressMsg += " ✅"
	}

	// Use log.Info for consistent styling with verbose mode
	if pb.completed {
		Info(progressMsg)
	} else {
		// For non-completed progress, use fmt.Printf with \r for real-time updates
		fmt.Printf("\r%s", progressMsg)
	}
}

// formatDuration formats duration to human readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

// Step represents a single step in a multi-step process
type Step struct {
	Name        string
	Description string
	Progress    *ProgressBar
	StartTime   time.Time
	EndTime     time.Time
	Completed   bool
	Error       error
}

// MultiStepProgress manages multiple steps with progress tracking
type MultiStepProgress struct {
	steps       []*Step
	currentStep int
	title       string
}

// NewMultiStepProgress creates a new multi-step progress tracker
func NewMultiStepProgress(title string) *MultiStepProgress {
	return &MultiStepProgress{
		title: title,
		steps: make([]*Step, 0),
	}
}

// AddStep adds a new step to the progress tracker
func (msp *MultiStepProgress) AddStep(name, description string) {
	step := &Step{
		Name:        name,
		Description: description,
	}
	msp.steps = append(msp.steps, step)
}

// StartStep starts the specified step
func (msp *MultiStepProgress) StartStep(stepIndex int) {
	if stepIndex >= len(msp.steps) {
		return
	}

	msp.currentStep = stepIndex
	step := msp.steps[stepIndex]
	step.StartTime = time.Now()

	if !quiet {
		icon := getStepIcon(stepIndex)
		if verbose {
			// Verbose mode: show detailed start message
			Info(fmt.Sprintf("%s %s (starting...)", icon, step.Description))
		} else {
			// Default mode: show start message with consistent INFO styling
			progressStart(fmt.Sprintf("%s %s... ", icon, step.Description))
		}
	}
}

// CompleteStep completes the current step
func (msp *MultiStepProgress) CompleteStep(stepIndex int) {
	if stepIndex >= len(msp.steps) {
		return
	}

	step := msp.steps[stepIndex]
	step.EndTime = time.Now()
	step.Completed = true

	if !quiet {
		elapsed := step.EndTime.Sub(step.StartTime)
		icon := getStepIcon(stepIndex)
		if verbose {
			// Verbose mode: show complete log entry
			Info(fmt.Sprintf("%s %s ✅ (%s)", icon, step.Description, formatDuration(elapsed)))
		} else {
			// Default mode: complete the same line with consistent styling
			progressComplete(fmt.Sprintf("✅ (%s)", formatDuration(elapsed)))
		}
	}
}

// FailStep marks a step as failed
func (msp *MultiStepProgress) FailStep(stepIndex int, err error) {
	if stepIndex >= len(msp.steps) {
		return
	}

	step := msp.steps[stepIndex]
	step.EndTime = time.Now()
	step.Error = err

	if !quiet {
		icon := getStepIcon(stepIndex)
		if verbose {
			// Verbose mode: show complete log entry
			Error(fmt.Sprintf("%s %s ❌ Failed: %v", icon, step.Description, err))
		} else {
			// Default mode: complete the same line with failure styling
			progressFail(fmt.Sprintf("❌ Failed: %v", err))
		}
	}
}

// Complete completes the entire multi-step process
func (msp *MultiStepProgress) Complete() {
	if quiet {
		return
	}

	totalTime := time.Duration(0)
	for _, step := range msp.steps {
		if step.Completed {
			totalTime += step.EndTime.Sub(step.StartTime)
		}
	}

	Info(fmt.Sprintf("✅ %s completed in %s!", msp.title, formatDuration(totalTime)))
}

// getStepIcon returns an icon for the step based on its index
func getStepIcon(stepIndex int) string {
	// Icons corresponding to: VMs, Environment, Master, Workers, CNI, CSI, LoadBalancer, Kubeconfig
	icons := []string{"📦", "⚙️ ", "🔧", "🔗", "🌐", "💾", "⚖️ ", "📋"}
	if stepIndex < len(icons) {
		return icons[stepIndex]
	}
	return "▶️"
}

// Simple progress functions for backward compatibility

// ProgressInfo shows progress information (only in non-quiet mode)
func ProgressInfo(args ...any) {
	if !quiet {
		Info(args...)
	}
}

// ProgressInfof shows formatted progress information (only in non-quiet mode)
func ProgressInfof(format string, args ...any) {
	if !quiet {
		Infof(format, args...)
	}
}

// QuietInfo shows info only in verbose mode, hidden in quiet mode
func QuietInfo(args ...any) {
	if verbose {
		Info(args...)
	}
}

// QuietInfof shows formatted info only in verbose mode, hidden in quiet mode
func QuietInfof(format string, args ...any) {
	if verbose {
		Infof(format, args...)
	}
}

// pProgressStart shows progress start message with consistent INFO styling but without newline
func progressStart(args ...any) {
	if quiet {
		return
	}

	timeStr := time.Now().Format(timeFormat)
	coloredPrefix := getLevelPrefix("INFO", INFO)
	fmt.Fprintf(os.Stdout, "[%s] %s: %s", timeStr, coloredPrefix, fmt.Sprint(args...))
}

// progressComplete completes the progress line with consistent styling
func progressComplete(args ...any) {
	if quiet {
		return
	}

	message := fmt.Sprint(args...)
	fmt.Printf("%s\n", message)
}

// ProgressFail completes the progress line with failure message
func progressFail(args ...any) {
	if quiet {
		return
	}

	message := fmt.Sprint(args...)
	fmt.Printf("%s\n", message)
}
