# Scripts

Automation scripts for GamePanel development, testing, and deployment.

## 📁 Directory Structure

- [build/](build/) - Build and compilation scripts
- [deploy/](deploy/) - Deployment scripts
- [dev/](dev/) - Development utility scripts
- [test/](test/) - Testing scripts
- [diagnostics/](diagnostics/) - Diagnostic and health check scripts
- [cleanup/](cleanup/) - Cleanup and rollback scripts
- [install/](install/) - Installation scripts
- [utilities/](utilities/) - Utility scripts

## 🚀 Usage

### Development
```bash
# Start development environment
./scripts/dev/start-dev.sh

# Stop development environment
./scripts/dev/stop-dev.sh

# Format code
./scripts/dev/format.sh

# Lint code
./scripts/dev/lint.sh
```

### Testing
```bash
# Run tests
./scripts/test/test.sh

# Validate migrations
./scripts/test/validate_migrations.sh
```

### Deployment
```bash
# Deploy to production
./scripts/deploy/deploy-prod.sh
```

### Diagnostics
```bash
# Run diagnostics
./scripts/diagnostics/diagnose.sh

# Check health
./scripts/diagnostics/healthcheck.sh
```
