#!/bin/bash
set -e

echo "Deploying Nginx test application..."
kubectl apply -f nginx-deployment.yaml
kubectl apply -f nginx-service.yaml

echo "Waiting for deployment to be ready..."
kubectl wait --for=condition=available --timeout=60s deployment/nginx

echo "Checking service status..."
kubectl get service nginx

echo "Waiting for LoadBalancer IP to be assigned..."
while [ -z "$(kubectl get service nginx -o jsonpath='{.status.loadBalancer.ingress[0].ip}')" ]; do
  echo "Waiting for IP assignment..."
  sleep 5
done

IP=$(kubectl get service nginx -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "LoadBalancer IP assigned: $IP"
echo "You can test the service by accessing: http://$IP"

# Optional: Test connectivity
if command -v curl &> /dev/null; then
  echo "Testing connection to Nginx..."
  curl -s --connect-timeout 5 http://$IP | grep -o "<title>.*</title>"
fi
