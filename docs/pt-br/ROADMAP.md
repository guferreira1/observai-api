# Roteiro (Roadmap)

Este roadmap lista capacidades que ficaram fora do escopo inicial de entrega e próximos
aperfeiçoamentos de plataforma.

## Suporte de provedores planejado

- Suporte adicional de LLM: Gemini (`generateContent`).
- Provedores de observabilidade com superfície de API mais complexa.

## Reforço de produto

- Acompanhamento de custo e consumo de tokens por provedor LLM.
- Isolação por tenant e RBAC específica por tenant.
- Score de regressão de prompts com conjunto de casos de referência.
- Chart Helm oficial e presets completos para Kubernetes.
- Playbooks de backup e restore para PostgreSQL/Redis em produção.

## Melhorias de plataforma

- Dashboards para profundidade da fila, backlog e uso de tokens por análise.
- Exportações/alertas adicionais para falhas de webhook.
- Melhor invalidação de cache para análises longas e contexto expirado.

## Restrições de engenharia preservadas

- Manter adapters de provedores substituíveis e com capacidades bem delimitadas.
- Manter separação por capacidade em provedores com múltiplos recursos.
- Manter `factory.Dependencies` como único ponto de injeção de dependências compartilhadas.
- Manter o mesmo pipeline de auth/middleware; evitar sistemas paralelos de autenticação.

