# Cilium Star Wars Demo Test

This directory contains scripts and configuration files for testing the Cilium network policy functionality, based on the official Cilium Star Wars demo example.

## Overview

Cilium is an open-source software used to provide and transparently secure the network connectivity and load balancing between application workloads. It uses eBPF technology to achieve efficient network policies, observability, and security features.

This test suite demonstrates how to:
1. Deploy sample applications (Death Star service, TIE Fighters, and X-Wing Fighters)
2. Test network access by default
3. Apply fine-grained L7 HTTP network policies
4. Verify that the policies work as expected

## Test Content

The test script `test-starwars-demo.sh` will perform the following actions:

1. Check if Cilium is running in the cluster
2. Deploy the Star Wars demo application (Death Star service, TIE Fighter, and X-Wing pods)
3. Test the initial network access status (no policy restrictions)
4. Apply an L7 HTTP network policy to restrict TIE Fighters (Empire organization) to only send POST requests to the `/v1/request-landing` interface of the Death Star
5. Conduct three tests:
   - Test if TIE Fighters can perform the allowed POST requests (should succeed)
   - Test if TIE Fighters cannot perform the forbidden PUT requests (should be rejected)
   - Test if X-Wing Fighters (non-Empire organization) cannot access the Death Star at all (should time out)
6. Clean up all resources after the test is complete

## Prerequisites

- A running Kubernetes cluster
- Cilium installed and running (minimum version 1.8)
- The `kubectl` command available, configured to connect to the cluster

## Usage

1. Ensure that Cilium is running in your cluster:
   ```
   kubectl get pods -n kube-system -l k8s-app=cilium
   ```

2. Run the test script:
   ```
   chmod +x test-starwars-demo.sh
   ./test-starwars-demo.sh
   ```

3. Observe the output to verify that all tests pass

## File Structure

```
tests/cilium/
├── README.md                      # This document
├── test-starwars-demo.sh          # Main test script
└── manifests/
    └── starwars/
        ├── http-sw-app.yaml       # Application deployment configuration
        └── sw_l3_l4_l7_policy.yaml # Cilium L7 network policy
```

## References

- [Cilium Star Wars Demo Documentation](https://docs.cilium.io/en/latest/gettingstarted/demo/)
- [Cilium Network Policy](https://docs.cilium.io/en/latest/security/policy/)
- [Cilium HTTP Policy Example](https://docs.cilium.io/en/latest/security/policy/language/#http) ****