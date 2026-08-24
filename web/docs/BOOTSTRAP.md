# Bootstrap Setup

This document records the bootstrap configuration for the Korp frontend application.

## Tooling

- **Framework:** Angular 22
- **Package Manager:** Bun
- **Styles:** SCSS
- **Tests:** Vitest (via Angular CLI)

## Package Management Policy

We use **Bun** for dependency installation and running scripts.
- Use `bun install` to install dependencies.
- `bun.lock` is committed to version control.
- `package-lock.json`, `yarn.lock`, or `pnpm-lock.yaml` are prohibited.

## Local Commands

- `bun run start`: Starts the local development server (Angular CLI).
- `bun run test`: Executes unit tests via Vitest.
- `bun run lint`: Lints the application (Angular ESLint is already configured).
- `bun run build`: Builds the production bundle.

## Delivery Structure

This repository is intended to become the `/web` directory in the final `Korp_Teste_SeuNome` delivery repository.

## API URL Configuration

API base URLs are managed via `src/environments/environment.ts`.
Stock and Billing services are separate domains. Development ports are 8081 for Stock and 8082 for Billing.

## Angular Material

Angular Material is installed with a custom theme. Material components are imported on a per-use basis in standalone components to maintain optimal bundle sizes.

## Lint and Quality

The repository is configured to use Lefthook for pre-commit checks (fast) and pre-push checks (heavier verification) at the root level.
