# Hexagonal feature skill

Use this playbook when adding a new feature.

## Steps

1. Define the use case in plain language.
2. Identify domain models and value objects.
3. Define required inbound and outbound ports.
4. Implement core use case logic without framework dependencies.
5. Implement inbound adapter mapping.
6. Implement outbound adapter mapping.
7. Add tests for use case behavior.
8. Add adapter tests when mapping logic is relevant.
9. Review observability, security and performance implications.

## Output expectation

The final design must allow provider replacement without changing the core rule.
