# Contributing to GamePanel

## Getting Started
1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Development Setup
See [docs/development/development.md](docs/development/development.md) for the full development guide.

## Code Style
- Go: Use `gofmt` and `goimports` (run `make format`)
- TypeScript: Use Prettier (run `npx prettier --write`)
- Follow existing patterns in the codebase
- Keep functions focused and small
- Write tests alongside code changes

## Commit Messages
- Use conventional commits: `feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`
- Keep messages clear and descriptive
- Reference issues and PRs when applicable

## Pull Request Process
1. Ensure all tests pass (`make test`)
2. Ensure code is formatted (`make format`)
3. Update documentation if needed
4. Add tests for new features
5. Get at least one review before merging

## Testing Expectations
- Backend: `(cd forge/api && go test ./...)` and `(cd beacon && go test ./...)`
- Frontend: `npm run typecheck`, `npm test`
- Lint: `npm run lint`
- Build: `npm run build`

## Security
- Report security-sensitive problems privately to the repository owner
- Do not publish credentials or exploit details in public issues
- Never commit secrets, tokens, or passwords to the repository
- Use `infra/gen-env.sh` or `infra/gen-env.ps1` to generate production secrets

## Code of Conduct
- Be respectful and inclusive
- Focus on constructive feedback
- Assume good intentions
