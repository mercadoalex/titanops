# Estrategia de Testing y Validación — TitanOps

> Cómo probar todo el proyecto de forma sustentable, sin gastar de más, y con datos reales.

---

## El Problema

TitanOps es una plataforma de observabilidad autónoma para Kubernetes. Para validar que funciona necesitamos:

1. **Un cluster de K8s** — cuesta dinero (EKS ~$75/mes solo el control plane + nodos)
2. **Aplicaciones corriendo** — sin workloads no hay señales que observar
3. **Datos/actividad real** — alertas, eventos, métricas, deploys, pod crashes
4. **Reproducibilidad** — poder repetir escenarios de incidente a demanda

Sin esto, estamos construyendo a ciegas.

---

## Estrategia en 4 Capas

El principio: **maximizar cobertura de testing con el mínimo costo posible**. Cada capa cubre más realismo pero cuesta más. Usamos las capas baratas para el 90% del desarrollo y la capa cara solo para validación final.

```
┌─────────────────────────────────────────────────────────┐
│  Capa 4: Cluster EKS real (validación final)            │  $$$  — Solo cuando necesitas
├─────────────────────────────────────────────────────────┤
│  Capa 3: kind/k3d local con app demo (integration)      │  $0   — Semanal
├─────────────────────────────────────────────────────────┤
│  Capa 2: Eval runner con escenarios YAML (regression)   │  $0   — Cada PR
├─────────────────────────────────────────────────────────┤
│  Capa 1: Unit tests + mocks (desarrollo diario)         │  $0   — Cada commit
└─────────────────────────────────────────────────────────┘
```

---

## Capa 1: Unit Tests + Mocks (ya existe parcialmente)

**Costo: $0**
**Cuándo: cada commit, CI automático**

Lo que ya tienes:
- Tests en `correlation/` con race detector
- Eval runner con escenarios YAML para correlation y earthworm
- CI en GitHub Actions (lint + build + test)

Lo que falta agregar:
- Tests unitarios para cada integration receiver (AlertManager ya tiene)
- Mocks del correlation engine para testear mappers aislados
- Tests de las funciones de mapping puras (input → export.Event)

**Principio:** Todo mapper y receiver se testea con datos hardcodeados. No necesitas K8s para validar que un JSON de AlertManager se convierte correctamente en `export.Event`.

---

## Capa 2: Eval Runner Expandido (escenarios de incidente)

**Costo: $0**
**Cuándo: cada PR, CI automático**

El eval runner ya funciona para correlation. La idea es expandirlo:

```yaml
# eval/scenarios/integration/alertmanager_to_correlation.yaml
name: "AlertManager alerts correlate into single incident"
description: "5 AlertManager alerts from same node in 30s window → 1 incident"
inputs:
  source: alertmanager
  webhook_payloads:
    - file: testdata/alertmanager_node_disk.json
    - file: testdata/alertmanager_pod_crash.json
    - file: testdata/alertmanager_high_latency.json
  config:
    time_window_seconds: 60
    confidence_threshold: 60
expected:
  incidents_generated: 1
  min_confidence: 70
  modules_involved: ["alertmanager"]
```

Esto prueba el flujo completo (webhook → mapping → correlation) sin red ni cluster.

### Datos de test realistas

Crear `integrations/testdata/` con payloads reales capturados de:
- AlertManager webhook (formato documentado públicamente)
- Falco http_output (formato documentado)
- Argo CD notifications (formato documentado)
- K8s Events API (formato estándar)

Estos se obtienen una sola vez de la documentación oficial y se guardan como fixtures.

---

## Capa 3: Cluster Local con App Demo (EL MÁS IMPORTANTE)

**Costo: $0 (solo tu laptop)**
**Cuándo: desarrollo semanal, antes de cada release**

### La Solución: `kind` + microservicio demo + generador de caos

En lugar de pagar EKS, levantamos un cluster local con [kind](https://kind.sigs.k8s.io/) (Kubernetes IN Docker) y desplegamos una app demo que genera señales reales.

### Componentes

```
scripts/localdev/
├── kind-config.yaml           # Cluster local de 3 nodos
├── setup.sh                   # Levanta todo de un comando
├── teardown.sh                # Destruye todo limpio
├── demo-app/
│   ├── Dockerfile             # App demo multi-servicio
│   ├── deployment.yaml        # 3 servicios: api, worker, db
│   └── main.go                # Genera métricas, logs, errores periódicos
├── chaos/
│   ├── scenarios.yaml         # Escenarios de caos configurable
│   ├── pod-killer.sh          # Mata pods aleatorios
│   ├── latency-injector.yaml  # NetworkPolicy que introduce latencia
│   └── resource-hog.yaml      # Pod que consume CPU/memoria hasta OOM
└── monitoring/
    ├── prometheus-values.yaml # Prometheus + AlertManager minimal
    └── alertmanager-config.yaml  # Reglas que disparan alertas hacia TitanOps
```

### La App Demo: "payments-demo"

Un microservicio Go simple que simula un servicio de pagos:

```go
// 3 deployments en K8s:
// - payments-api (HTTP, recibe requests, a veces falla)
// - payments-worker (consume de una queue, a veces se cuelga)
// - payments-db (PostgreSQL ligero, a veces se llena el disco)

// Comportamiento configurable:
// ERROR_RATE=0.05        → 5% de requests fallan con 500
// LATENCY_P99_MS=2000    → cola pesada de latencia
// OOM_AFTER_MINUTES=10   → leak de memoria que causa OOM kill
// CRASH_EVERY_MINUTES=5  → panic periódico (simula bug)
```

**Por qué esto funciona:** El app genera señales reales — pod restarts, OOMKilled events, high latency metrics, error rate spikes — que TitanOps observa y correlaciona exactamente como en producción.

### Flujo de un test local end-to-end

```bash
# 1. Levantar cluster + app + monitoring
make localdev-up

# 2. Desplegar TitanOps en el cluster local
helm install titanops ./helm/titanops -f helm/titanops/values-local.yaml

# 3. Inyectar caos (pod crash + alertas + deploy)
make localdev-chaos scenario=node-pressure

# 4. Verificar que TitanOps correlacionó
curl localhost:8080/api/correlations | jq .

# 5. Limpiar
make localdev-down
```

### Costos reales: $0

- kind corre en Docker en tu Mac
- La app demo son containers triviales (~50MB)
- Prometheus + AlertManager son ligeros en modo local
- Todo se destruye con un comando

### kind cluster config

```yaml
# scripts/localdev/kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
  - role: worker
  # 3er nodo opcional para simular topología multi-nodo
  - role: worker
    labels:
      topology.kubernetes.io/zone: "us-east-2b"
```

3 workers es suficiente para probar correlación cross-node, scheduling, y node pressure.

---

## Capa 4: EKS Real (validación pre-release)

**Costo: ~$100-150/mes cuando está activo**
**Cuándo: solo para validación final, se apaga después**

### Estrategia de costo mínimo

Tu infra Terraform ya está configurada con `m5.xlarge` (3 nodos deseados). Optimización:

| Cambio | Ahorro |
|--------|--------|
| Usar `t3.medium` para testing (en vez de `m5.xlarge`) | ~60% menos en nodos |
| `node_desired_size = 2` (mínimo viable) | Menos nodos |
| Apagar cluster cuando no se usa (terraform destroy) | Solo pagas cuando pruebas |
| Usar Spot Instances para nodos worker | ~70% descuento |

### Cómo minimizar tiempo encendido

```bash
# Día de validación (1-2 días al mes máximo):
make infra-up          # terraform apply (~15 min)
make deploy-full       # helm install + demo app + chaos
make validate          # correr suite completa de validación
make infra-down        # terraform destroy — dejas de pagar
```

**Costo real estimado si solo enciendes 2-3 días/mes:**
- EKS control plane: $0.10/hr × 72hr = $7.20
- 2× t3.medium spot: ~$0.013/hr × 2 × 72hr = $1.87
- NATS (in-cluster): $0
- CockroachDB Serverless (free tier): $0
- **Total: ~$10-15/mes**

### Cuándo SÍ necesitas EKS real

- Validar eBPF/kernel features (Earthworm, Quack) — requiere kernel real, no funciona en kind
- Performance testing bajo carga
- Validar networking real (CNI, NetworkPolicies)
- Demo a stakeholders/inversores

---

## El Generador de Datos: "titanops-chaos"

El componente clave que resuelve tu problema de "necesito datos reales":

```go
// cmd/titanops-chaos/main.go
// Un generador de incidentes configurable que simula escenarios de producción

// Modes:
// 1. "steady" — tráfico normal con errores ocasionales (background noise)
// 2. "incident" — escenario específico (deploy-broke-prod, cascade-failure, etc.)
// 3. "replay" — reproduce un incidente real capturado previamente

// Scenarios predefinidos:
// - deploy-broke-prod: Argo CD sync → error rate spike → pod OOMKill
// - cascade-failure: Node pressure → pod evictions → service degradation
// - security-breach: Falco alert (exec in container) + network anomaly
// - alert-storm: 20 AlertManager alerts en 30s (alert fatigue)
```

### Escenarios como YAML (reutilizables entre local y EKS)

```yaml
# scripts/localdev/chaos/scenarios/deploy-broke-prod.yaml
name: deploy-broke-prod
description: "Simula un deploy que rompe producción"
steps:
  - action: argocd_sync
    app: payments-api
    revision: "bad-commit-abc123"
    delay: 0s
  - action: inject_errors
    target: payments-api
    error_rate: 0.80
    delay: 10s
  - action: trigger_alerts
    alerts:
      - alertname: HighErrorRate
        severity: critical
        namespace: payments
      - alertname: PodCrashLooping
        severity: warning
        pod: payments-api-xyz
    delay: 30s
  - action: pod_oom
    target: payments-worker
    delay: 45s
expected_titanops_response:
  incidents: 1
  confidence_min: 75
  modules_involved: ["alertmanager", "k8sevents", "argocd"]
  actions_triggered: ["isolate_pod"]
```

---

## Datos Reales sin Cluster: Replay Mode

Para desarrollo diario sin levantar nada:

1. **Capturar** señales reales cuando SÍ tengas el cluster encendido
2. **Guardarlas** como fixtures en `eval/captures/`
3. **Reproducirlas** contra el correlation engine en modo offline

```bash
# Mientras el cluster está vivo, capturar todo:
make capture-signals duration=1h output=eval/captures/2024-incident-cascade/

# Después, sin cluster, reproducir:
make replay capture=eval/captures/2024-incident-cascade/
```

Esto es oro: captures señales una vez, las usas infinitas veces para regresión.

---

## Plan de Implementación

### Fase 1 — Inmediata (esta semana, $0)

| Tarea | Esfuerzo | Impacto |
|-------|----------|---------|
| Crear `scripts/localdev/` con kind + setup script | 1 día | Alto — habilita testing local |
| Crear app demo "payments-demo" (3 deployments simples) | 1 día | Alto — genera señales |
| Agregar Prometheus + AlertManager minimal al cluster local | 0.5 día | Medio — alimenta AlertManager receiver |
| Crear 3 escenarios de caos básicos (pod crash, OOM, alert storm) | 0.5 día | Alto — datos para correlación |
| Agregar `make localdev-up` / `make localdev-down` | 0.5 día | Alto — DX |

### Fase 2 — Semana siguiente ($0)

| Tarea | Esfuerzo | Impacto |
|-------|----------|---------|
| Expandir eval runner para soportar integration scenarios | 1 día | Medio |
| Crear testdata con payloads reales (AlertManager, Falco, ArgoCD, K8s Events) | 1 día | Alto |
| Signal capture/replay system básico | 1 día | Alto — testing sin cluster |
| CI: agregar job que levanta kind + corre integration tests | 1 día | Medio |

### Fase 3 — Cuando necesites validación real (~$10-15)

| Tarea | Esfuerzo | Impacto |
|-------|----------|---------|
| Agregar variable Terraform para Spot Instances | 0.5 día | Ahorro de ~70% en nodos |
| Crear `make infra-up` / `make infra-down` con protección | 0.5 día | Evita dejar cluster prendido |
| Script de validación completa E2E en EKS | 1 día | Confidence para release |
| Budget alert en AWS ($20/mes) | 10 min | Protección contra sorpresas |

---

## Resumen de Costos

| Actividad | Frecuencia | Costo |
|-----------|-----------|-------|
| Desarrollo diario (unit tests, eval runner) | Diario | $0 |
| Testing local con kind + app demo | Semanal | $0 |
| CI en GitHub Actions | Cada PR | $0 (free tier) |
| Validación en EKS real (2-3 días/mes) | Mensual | ~$10-15 |
| **Total mensual estimado** | | **~$10-15** |

Comparado con dejar un cluster EKS encendido 24/7: **~$200-400/mes**. Ahorro del ~95%.

---

## Decisiones Arquitectónicas

1. **kind sobre minikube** — más ligero, multi-nodo real, mejor para testing de correlación cross-node
2. **App demo propia sobre microservices-demo de Google** — más control, más ligera, escenarios específicos para TitanOps
3. **Chaos propio sobre LitmusChaos/ChaosMesh** — evita dependencias pesadas, escenarios específicos, se integra con el eval runner
4. **Spot Instances para EKS** — los nodos pueden morir, pero para testing eso es irrelevante (e incluso genera señales interesantes)
5. **Terraform destroy entre sesiones** — el estado se guarda en S3 backend, rebuild tarda ~15 min, el ahorro justifica la espera

---

## Notas

- El módulo Earthworm (eBPF) NO funciona en kind (requiere kernel real). Se testea con mocks localmente y se valida solo en EKS.
- El módulo Quack (sched_ext) requiere kernel 6.12+. Misma situación: mock local, validación en EKS con AMI custom.
- Los receivers de integraciones (AlertManager, Falco, ArgoCD) SÍ funcionan 100% en kind — no dependen del kernel.
- El correlation engine es pure logic — funciona 100% sin cluster.
