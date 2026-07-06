# Contributing Guide

Welcome to the `appconfig-cache` project! This document guides you through setting up your local development environment, validating code quality, committing changes, and adhering to our software architecture guidelines.

---

## 🛠️ Local Environment Setup

To ensure code consistency across all contributions, we use **Lefthook** for Git hooks management and **goimports** for import layout organization.

1. **Verify Go is installed:** Go version `1.26+` is recommended.
2. **Install tooling and activate Git Hooks:**
   Run the following command at the root of the repository:
   ```bash
   make setup
   ```
   This target automatically:
   - Downloads and installs `lefthook` and `goimports` locally in your `GOPATH/bin`.
   - Initializes Git hooks for pre-commit (formatting, imports, linting) and commit-msg (Conventional Commits validation).

---

## 🔄 Local Verification and Development Loop

Before opening a Pull Request, run the local verification suite to ensure all validations pass:

### 1. Formatter and Linter
- **Format code:** `go fmt ./...`
- **Organize imports:** `$(go env GOPATH)/bin/goimports -w -local github.com/sousapedro11/appconfig-cache .`
- **Static Linter:** `golangci-lint run`

### 2. Unit Testing
All tests in this repository are **100% hermetic**: they run completely in-memory, without dependencies on external running Valkey (Redis) or AWS services. Mocks and stubs are injected at package levels.
- **Run tests and measure coverage:**
  ```bash
  go test -race -coverprofile=coverage.out -covermode=atomic ./... && go tool cover -func=coverage.out
  ```

---

## 💬 Conventional Commits Specification

Every commit message in this project is verified by the `commit-msg` hook to ensure it follows the **Conventional Commits** specification:

```text
<type>(<optional scope>): <verb in imperative mood> <description>
```

### Allowed Types
- **`feat`**: A new feature.
- **`fix`**: A bug fix.
- **`refactor`**: Code changes that neither fix a bug nor add a feature.
- **`docs`**: Documentation updates.
- **`test`**: Adding missing tests or correcting existing tests.
- **`chore`**: Maintenance, build configs, dependencies, or tool settings (e.g., Lefthook updates).
- **`build`**: Changes affecting the build system or external packages.
- **`ci`**: CI configuration file updates (e.g., GitHub Actions workflows).
- **`style`**: Changes that do not affect the meaning of the code (formatting, white-space, imports grouping).

*Examples:*
- `feat(cmd): implement Standalone HTTP server`
- `style: format imports and group dependencies`
- `chore(lefthook): fix sequential validation runner`

---

## 🎨 Software Coding Conventions

Our codebase is designed to be minimal, clean, and highly robust. Please follow these conventions:

1. **Object Calisthenics & Clean Code:**
   - **No `else` keyword:** Minimize nesting and write flatter, cleaner logic by using guard clauses and early returns.
   - **One level of indentation:** Keep functions small and single-purpose.
2. **Strong Typing:**
   - Avoid using empty interfaces (`interface{}` or `any`) unless strictly necessary for generic payloads or serializations.
3. **YAGNI & comment rule:**
   - Always prioritize simplicity. Avoid over-engineering or preparing code for future unrequested use cases.
   - **Comment decisions:** Use comment annotations to justify minimal, optimized, or YAGNI-based design decisions.
4. **Hermetic and Fast Tests:**
   - Unit tests must run instantly. Avoid external docker containers or active network socket listening in tests. Use standard interface mocks and package-level function stubs.
