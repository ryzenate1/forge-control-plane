# Contributing to GamePanel

## Getting Started
1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Development Setup
See [docs/development.md](docs/development.md)

## Code Style
- Go: Use `gofmt` and `goimports` (run `make format`)
- TypeScript: Use Prettier (run `npx prettier --write`)
- Follow existing patterns in the codebase

## Commit Messages
- Use conventional commits: `feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`
- Keep messages clear and descriptive

## Pull Request Process
1. Ensure all tests pass (`make test`)
2. Ensure code is formatted (`make format`)
3. Update documentation if needed
4. Add tests for new features
5. Get at least one review before merging
