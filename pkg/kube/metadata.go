package kube

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/log"
)

// ApplyNodeMetadata applies labels, annotations and taints to a Kubernetes node
func (k *Manager) ApplyNodeMetadata(nodeName string, metadata config.NodeMetadata) error {
	if len(metadata.Labels) == 0 && len(metadata.Annotations) == 0 && len(metadata.Taints) == 0 {
		log.Debugf("No metadata to apply for node %s", nodeName)
		return nil
	}

	log.Debugf("Applying metadata to node %s: %d labels, %d annotations, %d taints",
		nodeName, len(metadata.Labels), len(metadata.Annotations), len(metadata.Taints))

	// Apply labels
	for key, value := range metadata.Labels {
		labelCmd := fmt.Sprintf("kubectl label node %s %s=%s --overwrite", nodeName, key, value)
		_, err := k.sshRunner.RunCommand(k.MasterNode, labelCmd)
		if err != nil {
			return fmt.Errorf("failed to apply label %s=%s to node %s: %w", key, value, nodeName, err)
		}
		log.Debugf("Applied label %s=%s to node %s", key, value, nodeName)
	}

	// Apply annotations
	for key, value := range metadata.Annotations {
		annotateCmd := fmt.Sprintf("kubectl annotate node %s %s=%s --overwrite", nodeName, key, value)
		_, err := k.sshRunner.RunCommand(k.MasterNode, annotateCmd)
		if err != nil {
			return fmt.Errorf("failed to apply annotation %s=%s to node %s: %w", key, value, nodeName, err)
		}
		log.Debugf("Applied annotation %s=%s to node %s", key, value, nodeName)
	}

	// Apply taints
	for _, taint := range metadata.Taints {
		var taintCmd string
		if taint.Value != "" {
			taintCmd = fmt.Sprintf("kubectl taint node %s %s=%s:%s --overwrite",
				nodeName, taint.Key, taint.Value, taint.Effect)
		} else {
			taintCmd = fmt.Sprintf("kubectl taint node %s %s:%s --overwrite",
				nodeName, taint.Key, taint.Effect)
		}
		_, err := k.sshRunner.RunCommand(k.MasterNode, taintCmd)
		if err != nil {
			return fmt.Errorf("failed to apply taint %s=%s:%s to node %s: %w",
				taint.Key, taint.Value, taint.Effect, nodeName, err)
		}
		if taint.Value != "" {
			log.Debugf("Applied taint %s=%s:%s to node %s", taint.Key, taint.Value, taint.Effect, nodeName)
		} else {
			log.Debugf("Applied taint %s:%s to node %s", taint.Key, taint.Effect, nodeName)
		}
	}

	log.Infof("✅ Successfully applied metadata to node %s", nodeName)
	return nil
}

// ConvertNodeTaintToMetadata converts config.NodeTaint slice to config.Taint slice
func ConvertNodeTaintToMetadata(nodeTaints []config.Taint) []config.Taint {
	var taints []config.Taint
	for _, nt := range nodeTaints {
		taints = append(taints, config.Taint{
			Key:    nt.Key,
			Value:  nt.Value,
			Effect: nt.Effect,
		})
	}
	return taints
}
