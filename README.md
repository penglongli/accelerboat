# accelerboat

![logo](logo/logo.png)

---

AccelerBoat is an accelerator for OCI image registry. It can speed up the pulling of images in the entire LAN and converge the pulling traffic of the central registry.

## Applicable scenarios

- Large-scale clusters pull images slowly, speed up
- Reuse existing OCI images on other nodes in the kubernetes cluster
- Converge the traffic of the central registry to save the public network bandwidth traffic cost when pulling from the public network registry
- Pulling the central registry across clouds/regions faces whitelist issues, providing unified pull traffic export capabilities
- Configuring Multiple User/Passwords

## Supported capabilities

- **Image-Discovery:** upports discovery of OCI images (Dockerd/Containerd) on each node. If an image exists on a node in the cluster, there is no need to pull the image from the central warehouse

- **Convergence-Request:** converge 3 types of requests for pulling images in the cluster: authentication request, manifest request, download layer request; reduce the pressure on the central warehouse and save the public network traffic for pulling

- **BitTorrent-Protocol:** convert the original HTTP image download to p2p (UDP) download through the BitTorrent protocol to speed up the image pull speed

- **Unify-Traffic-Export:** export the image pull traffic of all nodes in the cluster through the network proxy. Mainly for some restricted network environments that need to be whitelisted

- **Multiple-UserPassword:** user only need to configure a domain, multiple usernames and passwords 

## Getting Started

