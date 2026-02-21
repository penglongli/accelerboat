# accelerboat

![logo](docs/logo/logo.png)

---

AccelerBoat is an OCI image accelerator that speeds up image pulls across the entire LAN and converges pull traffic to a central registry.

## Applicable Scenarios

**Scenario 1**: Large-scale clusters pull images slowly — improve pull speed

**Scenario 2**: Pulling from a central registry across continents/regions/networks is slow — save cross–public-network bandwidth and traffic costs

**Scenario 3**: Pulling from a central registry across continents faces blacklist/whitelist allowlisting — provide unified pull traffic egress

**Scenario 4**: Users across continents use multiple image registries and want to configure multiple usernames/passwords, achieving nearby pulls via CNAME

**Scenario 5**: Reuse OCI images that already exist on other nodes in the cluster

## Architecture Overview

See [Architecture and Principles](./docs/0-architecture.md) for AccelerBoat’s detailed implementation.

![arch](./docs/images/arch-0-summary.png)

## Getting Started

### How to Install

Add the custom Helm repo locally and pull the AccelerBoat chart:

```bash
helm repo add accelerboat https://penglongli.github.io/accelerboat
helm pull accelerboat/accelerboat
```
