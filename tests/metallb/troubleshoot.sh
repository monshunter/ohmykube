#!/bin/bash
set -e

echo "=== MetalLB Troubleshooting ==="
echo

echo "1. Checking MetalLB components:"
kubectl get pods -n metallb-system
echo

echo "2. Checking IPAddressPool configuration:"
kubectl get ipaddresspools -n metallb-system -o yaml
echo

echo "3. Checking L2Advertisement configuration:"
kubectl get l2advertisements -n metallb-system -o yaml
echo

echo "4. Checking service status:"
kubectl get svc nginx -o yaml
echo

echo "5. Checking node network configuration:"
kubectl get nodes -o wide
echo

echo "6. Testing connectivity from within the cluster:"
kubectl exec -it curl -- curl -s --connect-timeout 5 http://192.168.64.200 | head -n 10
echo

echo "7. Checking MetalLB controller logs:"
kubectl logs -n metallb-system -l app=metallb,component=controller --tail=20
echo

echo "8. Checking MetalLB speaker logs (first speaker pod):"
SPEAKER_POD=$(kubectl get pods -n metallb-system -l app=metallb,component=speaker -o jsonpath='{.items[0].metadata.name}')
kubectl logs -n metallb-system $SPEAKER_POD --tail=20
echo

echo "9. Network route information from a node:"
NODE=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')
echo "Routes on node $NODE:"
kubectl debug node/$NODE -it --image=busybox -- ip route
echo

echo "=== Troubleshooting Complete ==="
