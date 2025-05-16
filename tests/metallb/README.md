# MetalLB Nginx Test

This directory contains resources to test MetalLB with a simple Nginx deployment.

## Prerequisites

1. MetalLB must be installed and configured in your cluster
2. The `cheap` address pool must be defined in the MetalLB configuration

## Files

- `nginx-deployment.yaml`: Deploys a basic Nginx server
- `nginx-service.yaml`: Creates a LoadBalancer service that uses MetalLB for IP allocation
- `deploy-test.sh`: Script to deploy and test the Nginx application

## Usage

1. Make sure you're in the `metallb/demo` directory:
   ```
   cd metallb
   ```

2. Run the deployment script:
   ```
   ./deploy-test.sh
   ```

3. The script will:
   - Deploy the Nginx application
   - Create a LoadBalancer service
   - Wait for MetalLB to assign an IP address
   - Display the assigned IP
   - Attempt to test connectivity to the Nginx server

4. You can access the Nginx server at the IP address displayed by the script

## Cleanup

To remove the test resources:

```
./clear.sh
```

## Troubleshooting

If the LoadBalancer IP is not assigned:

1. Check that MetalLB is running:
   ```
   kubectl get pods -n metallb-system
   ```

2. Check MetalLB logs:
   ```
   kubectl logs -n metallb-system -l app=metallb
   ```

3. Verify your address pool configuration:
   ```
   kubectl get ipaddresspools -n metallb-system
   ```

4. Make sure the IP range in your address pool is appropriate for your network
