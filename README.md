# accelerboat

![logo](docs/logo/logo.png)

---

AccelerBoat is an accelerator for OCI image registry. It can speed up the pulling of images in the entire LAN and converge the pulling traffic of the central registry.

## Applicable scenarios

- Large-scale clusters pull images slowly, speed up
- Reuse existing OCI images on other nodes in the kubernetes cluster
- Converge the traffic of the central registry to save the public network bandwidth traffic cost when pulling from the public network registry
- Pulling the central registry across clouds/regions faces whitelist issues, providing unified pull traffic export capabilities
- Configuring Multiple User/Passwords

## Getting Started

