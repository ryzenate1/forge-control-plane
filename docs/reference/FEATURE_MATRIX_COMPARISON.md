# Feature Matrix Comparison

**Date:** 2026-06-18  
**Purpose:** Detailed feature-by-feature comparison across all platforms  
**Platforms:** GamePanel, Pterodactyl, Pelican, PufferPanel, Wings

---

## Legend

- ✅ **Complete** - Fully implemented and tested
- ⚠️ **Partial** - Implemented but has gaps or not fully tested
- ❌ **Missing** - Not implemented
- 🔒 **Admin Only** - Available only to administrators
- 👤 **User Feature** - Available to end users
- 🤖 **Daemon Feature** - Runs on node daemon

---

## Core Infrastructure

| Feature | GamePanel | Pterodactyl | Pelican | PufferPanel | Notes |
|---------|-----------|-------------|---------|-------------|-------|
| **Locations/Regions** | ✅ | ✅ | ✅ | ✅ | GamePanel has both concepts |
| **Nodes** | ✅ | ✅ | ✅ | ✅ | All implement node management |
| **Node Heartbeat** | ✅ | ✅ | ✅ | ✅ | GamePanel: HMAC-signed, 30s interval |
| **Node Health Tracking** | ✅ | ⚠️ | ⚠️ | ⚠️ | GamePanel has advanced health classification |
| **Node Capacity Monitoring** | ✅ | ⚠️ | ⚠️ | ⚠️ | GamePanel tracks historical capacity |
| **Node Maintenance Mode** | ✅ | ✅ | ✅ | ❌ | GamePanel has drain + maintenance states |
| **Allocations (Ports)** | ✅ | ✅ | ✅ | ✅ | All implement IP:Port allocation |
| **Allocation Ranges** | ✅ | ✅ | ✅ | ✅ | Create multiple ports at once |
| **Allocation Aliases** | ✅ | ✅ | ✅ | ❌ | PufferPanel uses different model |
| **Multi-Node Support** | ✅ | ✅ | ✅ | ✅ | All support distributed nodes |

