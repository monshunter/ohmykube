# Image Cache Integration Requirements
## Overview
The image cache system is designed to improve the efficiency and reliability of image management in OhMyKube by implementing local caching and optimized distribution to target nodes.

## Key Features
- Local caching of images to avoid repeated downloads
- Support for multiple image sources (Docker, Podman, Helm, etc.)
- YAML-based index file to track cached images
- Metadata including version, architecture, download URL, file paths, and timestamps
- Automatic index updates when images are added or removed
- Upload cached images to target nodes via SSH
- Verify image existence before upload to avoid unnecessary transfers
- Support for resumable transfers and error recovery

## Image Cache Related Code
- @pkg/cache/image_manager.go
- @pkg/cache/image_discovery.go
- @pkg/cache/types.go
- @pkg/cache/tool_detector.go

## Integration with Existing Code
- @pkg/addons/plugins/cni/cilium.go
- @pkg/addons/plugins/cni/flannel.go
- @pkg/addons/plugins/csi/rook.go
- @pkg/addons/plugins/csi/local_path.go
- @pkg/addons/plugins/lb/metallb.go
- @pkg/kube/kube_manage.go