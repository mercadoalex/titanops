# Open Source Integrations Roadmap

> Objetivo: Conectar TitanOps con el ecosistema open source para atraer usuarios y beneficiarnos de comunidades existentes.

---

## Resumen Ejecutivo

| # | Integración | Esfuerzo | Audiencia potencial | Barrera de entrada para el usuario |
|---|---|---|---|---|
| 1 | AlertManager receiver | 1-2 días | Enorme | Agregar un webhook |
| 2 | MCP registry publish | 1 día | Creciente rápido | Copiar un JSON |
| 3 | K8s Event watcher | 2-3 días | Todo K8s | Ninguna (auto) |
| 4 | Falco adapter | 2 días | Grande (CNCF) | Un sidecar |
| 5 | Argo CD webhook | 1-2 días | Grande (GitOps) | Un webhook |
| 6 | Grafana datasource | 1 semana | Masiva | Instalar plugin |
| 7 | OTLP receiver | 3-4 días | Masiva | Config de pipeline |
| 8 | NATS case study | 1 día | Pequeña pero técnica | N/A |

**Recomendación:** Empezar por el 1 (AlertManager receiver) y el 2 (publicar MCP server). Son los de menor esfuerzo con mayor alcance. El AlertManager receiver convierte a TitanOps en algo que cualquier equipo de SRE puede probar en 5 minutos sin cambiar nada de su stack.

---

## 1. AlertManager Receiver

**Qué es:** Un webhook HTTP que recibe alertas de Prometheus AlertManager y las convierte en `export.Event` para el correlation engine.

**Por qué:** Todo el mundo tiene AlertManager y sufre alert fatigue. TitanOps les dice "esas 5 alertas son el mismo incidente".

**Implementación:**
- Handler HTTP en `gateway/` o módulo separado `integrations/alertmanager/`
- Parsear el formato webhook de AlertManager (JSON con `alerts[]`, `labels`, `annotations`)
- Mapear a `export.Event`: labels → node/pod/namespace, severity → severity, alertname → event_type
- Ingestar al correlation engine como si fuera un módulo más (`module: "alertmanager"`)
- Config: `ALERTMANAGER_WEBHOOK_ENABLED=true`, `ALERTMANAGER_WEBHOOK_PORT=9095`

**Entregable:** Un endpoint `/webhooks/alertmanager` que cualquier AlertManager puede usar como receiver.

**Valor para el usuario:** Correlación automática de alertas sin instalar eBPF ni cambiar su stack.

---

## 2. MCP Registry Publish

**Qué es:** Publicar el MCP server (`cmd/titanops-mcp`) en los registries de MCP (mcp.so, smithery.ai, awesome-mcp-servers).

**Por qué:** El ecosistema MCP está explotando. Hay poca competencia en K8s ops. Estar temprano = visibilidad.

**Implementación:**
- Crear entrada en mcp.so y smithery.ai
- Agregar al README instrucciones de configuración para Claude Desktop y Kiro
- Publicar en awesome-mcp-servers (GitHub PR)
- Crear un ejemplo de conversación: "investiga por qué payments-service está fallando"

**Entregable:** TitanOps aparece en búsquedas de "kubernetes MCP server" y "ops MCP tools".

**Valor para el usuario:** Dar a cualquier AI agent la capacidad de investigar incidentes K8s vía TitanOps.

---

## 3. K8s Event Watcher

**Qué es:** Un componente que observa el API server de Kubernetes y convierte eventos nativos (pod crashes, node conditions, HPA scaling) en `export.Event`.

**Por qué:** Zero config. Si TitanOps corre en un cluster, ya tiene acceso al API server. El usuario no tiene que hacer nada.

**Implementación:**
- Nuevo módulo `integrations/k8s-events/` o dentro del RuntimeKernel
- Watch de `Events` vía `client-go` informers
- Mapear: `reason` → event_type, `involvedObject` → pod/node/namespace, `type: Warning` → severity high
- Filtrar noise (eventos normales como Scheduled, Pulled) — solo los relevantes para correlación
- Integrar como fuente pasiva que siempre corre si está en cluster

**Entregable:** TitanOps correlaciona eventos nativos de K8s sin configuración adicional.

**Valor para el usuario:** Obtener correlación útil desde el minuto 0 sin Prometheus ni Falco.

---

## 4. Falco Adapter

**Qué es:** Un adaptador que consume alertas de Falco (CNCF Incubating, detección de amenazas en runtime) y las inyecta al correlation engine.

**Por qué:** Falco tiene miles de usuarios generando alertas de seguridad. TitanOps les da correlación cross-signal que Falco solo no puede hacer. "Falco detectó exec sospechoso + Earthworm detectó degradación en el mismo nodo = ataque en progreso."

**Implementación:**
- Opción A: Falco output plugin (Go) — más integrado pero requiere que el usuario configure Falco
- Opción B: Webhook receiver (como AlertManager) — Falco ya tiene `http_output`
- Mapear campos de Falco: `output_fields.container.id` → pod, `output_fields.k8s.ns.name` → namespace, `priority` → severity
- Module ID: `"falco"`

**Entregable:** Un receptor HTTP `/webhooks/falco` + documentación de cómo configurar `falco.yaml` para enviar a TitanOps.

**Valor para el usuario:** Correlación security+health+performance usando una herramienta que ya tienen.

---

## 5. Argo CD Webhook

**Qué es:** Un receiver para eventos de Argo CD (sync started, sync succeeded, health degraded) que alimentan el scoring de deployment risk de OllinAI.

**Por qué:** Los equipos usando Argo CD quieren saber "¿este deploy rompió algo?" — TitanOps responde correlando el deploy con señales de salud.

**Implementación:**
- Webhook receiver en `/webhooks/argocd`
- Parsear Argo CD notification events (JSON)
- Mapear: `app.status.sync.revision` → commit_sha, `app.metadata.name` → service, sync status → event_type
- Generar eventos `ollinai/deployment_risk` con metadata de deploy
- Tu scoring ya da bonus por deployment metadata — esto lo activa con datos reales

**Entregable:** Argo CD envía notifications a TitanOps → correlación deploy↔incidente automática.

**Valor para el usuario:** Respuesta instantánea a "¿fue el último deploy lo que rompió producción?"

---

## 6. Grafana Datasource Plugin

**Qué es:** Un plugin de Grafana que consulta el API gateway de TitanOps y permite visualizar correlaciones, acciones autónomas y audit trail dentro de Grafana.

**Por qué:** ~10M de usuarios de Grafana. Si apareces en el marketplace de plugins, la visibilidad es desproporcionada al esfuerzo.

**Implementación:**
- Grafana datasource plugin (TypeScript/React, scaffolded con `@grafana/create-plugin`)
- Consultar endpoints: `/api/correlations`, `/api/actions`, `/api/audit`, `/api/health`
- Paneles: timeline de correlaciones, tabla de acciones con confidence, audit log filtrable
- Publicar en grafana.com/plugins
- Bonus: publicar dashboards JSON pre-built en grafana.com/dashboards

**Entregable:** `grafana-titanops-datasource` instalable desde Grafana → la gente ve correlaciones sin salir de su herramienta.

**Valor para el usuario:** Visibilidad de decisiones autónomas de TitanOps junto con sus métricas existentes.

---

## 7. OTLP Receiver

**Qué es:** Un endpoint que recibe telemetría en formato OpenTelemetry Protocol (OTLP) y la convierte en `export.Event` para correlación.

**Por qué:** OpenTelemetry es el estándar emergente. Cualquiera con un OTel Collector puede enviar señales a TitanOps agregando un exporter en su pipeline.

**Implementación:**
- Opción A: OTLP gRPC/HTTP receiver directo en el gateway (usando `go.opentelemetry.io/collector` libs)
- Opción B: OpenTelemetry Collector exporter plugin custom (`titanopsexporter`)
- Convertir: spans con errors → high severity events, log records con level=ERROR → events, métricas con anomalías → events
- Preservar trace_id/span_id en Labels para poder trazar la correlación hasta el trace original

**Entregable:** TitanOps como destino en cualquier pipeline de OpenTelemetry.

**Valor para el usuario:** Sin cambiar su instrumentación, obtienen correlación inteligente de sus señales existentes.

---

## 8. NATS Case Study

**Qué es:** Documentar y publicar cómo TitanOps usa NATS como event bus para correlación cross-module en un cluster K8s.

**Por qué:** Te posiciona como referencia técnica en la comunidad NATS. Proyectos pequeños pero con opinión técnica fuerte ganan respeto rápido.

**Implementación:**
- Escribir un blog post / caso de uso: "Real-time K8s incident correlation with NATS"
- Incluir arquitectura, event schema (protobuf), throughput, por qué NATS y no Kafka
- Proponer al equipo de NATS incluirlo en su documentation/examples
- Publicar en su community forum y Discord

**Entregable:** Artículo publicado + PR a nats-io/nats.docs o community showcase.

**Valor para TitanOps:** Credibilidad técnica + backlinks + visibilidad en una comunidad que valora proyectos reales.

---

## Orden de Ejecución (Sprints)

### Sprint 1 — Semana 1 (Quick wins de máximo impacto)

| Día | Tarea |
|-----|-------|
| L-M | **AlertManager receiver** — implementar webhook handler + tests |
| M | **MCP registry publish** — crear entries en mcp.so, smithery.ai, PR a awesome-mcp-servers |
| J | **NATS case study** — escribir artículo, publicar |
| V | Testing end-to-end del AlertManager receiver con un AlertManager real |

### Sprint 2 — Semana 2 (Kubernetes nativo)

| Día | Tarea |
|-----|-------|
| L-M | **K8s Event watcher** — implementar con client-go informers |
| M-J | **Argo CD webhook** — implementar receiver + docs de config |
| V | Testing integrado: K8s events + Argo CD → correlation engine |

### Sprint 3 — Semana 3 (Security + Observability)

| Día | Tarea |
|-----|-------|
| L-M | **Falco adapter** — webhook receiver + mapeo de campos |
| M-V | **OTLP receiver** — implementar con las libs de OTel Collector |

### Sprint 4 — Semana 4 (La cereza: Grafana)

| Día | Tarea |
|-----|-------|
| L-V | **Grafana datasource plugin** — scaffold, implementar queries, panels, publicar |

---

## Estructura de Código Propuesta

```
titanops/
├── integrations/
│   ├── alertmanager/
│   │   ├── receiver.go          # HTTP webhook handler
│   │   ├── receiver_test.go
│   │   └── mapping.go           # AlertManager → export.Event
│   ├── falco/
│   │   ├── receiver.go          # HTTP webhook handler
│   │   ├── receiver_test.go
│   │   └── mapping.go           # Falco → export.Event
│   ├── argocd/
│   │   ├── receiver.go          # HTTP webhook handler
│   │   ├── receiver_test.go
│   │   └── mapping.go           # Argo CD → export.Event
│   ├── k8sevents/
│   │   ├── watcher.go           # client-go informer
│   │   ├── watcher_test.go
│   │   └── mapping.go           # K8s Event → export.Event
│   └── otlp/
│       ├── receiver.go          # OTLP gRPC/HTTP receiver
│       ├── receiver_test.go
│       └── mapping.go           # OTLP signals → export.Event
├── grafana-plugin/
│   ├── src/
│   │   ├── datasource.ts
│   │   ├── ConfigEditor.tsx
│   │   └── QueryEditor.tsx
│   ├── package.json
│   └── plugin.json
└── docs/
    ├── open-source-integrations-roadmap.md  ← este documento
    └── nats-case-study.md
```

---

## Criterio de Éxito por Integración

| Integración | Métrica de éxito |
|---|---|
| AlertManager receiver | Un usuario puede configurar AlertManager para enviar a TitanOps en <5 min |
| MCP registry | Aparece en búsquedas de "kubernetes" en mcp.so |
| K8s Event watcher | TitanOps genera correlaciones sin config adicional al desplegarse |
| Falco adapter | Demo funcional: alerta Falco + evento Earthworm = incidente correlado |
| Argo CD webhook | Deploy via Argo CD aparece como deployment_risk en correlaciones |
| Grafana datasource | Plugin instalable desde Grafana UI, muestra timeline de incidentes |
| OTLP receiver | OTel Collector envía traces/logs → TitanOps genera eventos |
| NATS case study | Publicado en community showcase o blog de NATS |

---

## Notas Arquitectónicas

- Todos los receivers siguen el mismo patrón: HTTP handler → mapping function → `engine.Ingest(ctx, event)`
- Los mappings son funciones puras y testeables independientemente
- Cada integración es opcional y se activa por variable de entorno
- Graceful degradation: si el receiver no arranca, TitanOps sigue funcionando con sus módulos internos
- Todos los receivers deben emitir métricas Prometheus (events_received_total, events_mapped_total, errors_total)

---

## Phase 2: Additional Alert Source Receivers (Low Priority)

> Estas integraciones se implementan DESPUÉS de que el producto core sea sólido.
> La prioridad es código robusto, limpio, con buenas prácticas — no más features.

| # | Integración | Esfuerzo | Tipo | Notas |
|---|---|---|---|---|
| 9 | Datadog Webhook receiver | 1 día | Inbound | Formato propietario, muchos equipos enterprise |
| 10 | Grafana Alerting receiver | 1 día | Inbound | Reemplazando AlertManager en muchos setups |
| 11 | Generic Webhook receiver | 2-3 días | Inbound | Mapping configurable via YAML — cubre cualquier plataforma |
| 12 | PagerDuty outbound | 1-2 días | Outbound | TitanOps → crear incidents en PagerDuty |
| 13 | OpsGenie outbound | 1 día | Outbound | TitanOps → crear alerts en OpsGenie |
| 14 | Dynatrace inbound | 1 día | Inbound | Receptor de alertas Dynatrace → correlation engine |

### Prerequisitos antes de implementar Phase 2

- [ ] Core product es estable y bien testeado
- [ ] Documentación de contribución clara (CONTRIBUTING.md)
- [ ] CI pipeline verde con linting, tests, y coverage
- [ ] Al menos 1-2 usuarios reales usando las integraciones de Phase 1
- [ ] Code review de la arquitectura de receivers por alguien externo

### Arquitectura propuesta

```
integrations/
├── alertmanager/     ← ✅ Hecho (cubre Prometheus, VictoriaMetrics, Thanos)
├── datadog/          ← Webhook receiver para Datadog monitors
├── grafana/          ← Webhook receiver para Grafana Alerting
├── generic-webhook/  ← Receptor configurable (mapping via YAML)
├── falco/            ← Phase 1
├── argocd/           ← Phase 1
├── k8sevents/        ← Phase 1
├── otlp/             ← Phase 1
└── outbound/
    ├── pagerduty/    ← TitanOps → PagerDuty
    ├── opsgenie/     ← TitanOps → OpsGenie
    └── slack/        ← TitanOps → Slack
```

### Nota sobre prioridades

No estamos haciendo castillos en el aire. El orden correcto es:
1. Producto sólido (código limpio, tests, docs)
2. Integraciones core que demuestran valor (Phase 1: AlertManager, K8s Events, Falco, Argo CD)
3. Publicación y feedback real de la comunidad
4. SOLO ENTONCES: más receivers para plataformas propietarias (Phase 2)

---

## Phase 3: K8sGPT Integration (Low Priority)

> K8sGPT es complementario, no competidor. Ellos explican, nosotros actuamos.
> Documentación completa: [k8sgpt-positioning-and-integration.md](k8sgpt-positioning-and-integration.md)

| # | Integración | Esfuerzo | Tipo | Notas |
|---|---|---|---|---|
| 15 | TitanOps como custom analyzer de K8sGPT | 2-3 días | Outbound | Exponer correlaciones/acciones en formato K8sGPT analyzer |
| 16 | K8sGPT findings como fuente de eventos | 1-2 días | Inbound | Consumir resultados de K8sGPT operator (CRDs o gRPC) y convertir a export.Event |

### Por qué esto importa

- K8sGPT tiene ~9k GitHub stars y es CNCF Sandbox — acceso a su audiencia sin esfuerzo
- Son complementarios: K8sGPT diagnostica a nivel API, TitanOps observa a nivel kernel y actúa
- Un usuario con ambos obtiene: diagnóstico (K8sGPT) + correlación + acción autónoma (TitanOps)

### Prerequisitos

- [ ] TitanOps publicado y funcional en un cluster real
- [ ] Al menos el AlertManager receiver y K8s Event watcher funcionando en producción
- [ ] README y docs orientados a la comunidad
- [ ] Clarity total en el messaging: "K8sGPT explains. TitanOps acts."
