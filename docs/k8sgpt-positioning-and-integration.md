# K8sGPT vs TitanOps — Posicionamiento e Integración

> Análisis competitivo y estrategia de integración con K8sGPT (CNCF Sandbox, ~9k stars).

---

## Comparativa

| Dimensión | K8sGPT | TitanOps |
|---|---|---|
| **Qué es** | Scanner/diagnosticador que usa LLMs para explicar problemas | Plataforma autónoma que detecta, decide y actúa |
| **Modo de operación** | Reactivo: "analiza mi cluster" → "tienes este problema" | Proactivo: observa 24/7, correlaciona, actúa sin que nadie lo pida |
| **Fuente de datos** | API de Kubernetes (pods, services, events) | eBPF (kernel-level) + K8s API |
| **Rol del AI** | Explica lo que encontró (post-hoc). Depende de LLMs cloud | Decide qué hacer (real-time). ONNX local, cloud opcional |
| **Acciones** | No actúa. Solo diagnostica y sugiere | Cordona nodos, aísla pods, renueva certs, rebalancea CPU |
| **Correlación** | No. Analiza cada recurso por separado | Cross-module: "CPU alta + honeytoken + deploy = ataque" |
| **eBPF** | No | Core del producto (cilium/ebpf, Tetragon, Aya, sched_ext) |
| **Seguridad** | `securityAnalyzer` básico | Deception engine (honeytokens, threat classification, pod isolation) |
| **MCP server** | 12 tools, 3 resources, 3 prompts | 6 tools enfocados en operaciones autónomas |
| **Madurez** | CNCF Sandbox, comunidad activa | Proyecto individual, pre-publicación |
| **Target user** | DevOps/SRE que quiere entender qué pasa | SRE que quiere que el sistema se arregle solo |

---

## La Diferencia Fundamental

**K8sGPT = AI que explica. TitanOps = AI que actúa.**

K8sGPT es esencialmente un "kubectl get problems --explain-in-english". Mira el estado del cluster vía API, encuentra problemas evidentes (CrashLoopBackOff, ImagePullBackOff, PVC pending), y le pide a un LLM que explique por qué.

TitanOps observa el kernel con eBPF, detecta anomalías que no son visibles desde la API de Kubernetes (patrones de heartbeat, accesos a honeytokens, scheduling inefficiencies), correlaciona señales cross-module, y ejecuta acciones autónomas.

Son capas diferentes del mismo problema:

```
K8sGPT:   "Tu pod está crasheando porque le falta memoria"
TitanOps: *ya detectó la anomalía hace 30 segundos, aisló el pod, y alertó al operador*
```

---

## Posicionamiento

### Lo que NO debemos decir

- "TitanOps — AI para Kubernetes" (suena idéntico a K8sGPT)
- "Diagnostica tu cluster con AI" (eso ya lo hace K8sGPT)

### Lo que SÍ debemos decir

- "Your observability stack tells you what's wrong. TitanOps fixes it."
- "K8sGPT tells you what's wrong. TitanOps fixes it before you even notice."
- "Autonomous remediation powered by eBPF + local AI"

### Diferenciadores clave a enfatizar

1. **eBPF-based observation** — K8sGPT no toca el kernel. TitanOps observa a nivel de kernel. Señales que la API de K8s no puede dar.
2. **Autonomous action** — K8sGPT no hace nada. Solo explica. TitanOps ejecuta: cordon, isolate, restart, renew.
3. **Cross-signal correlation** — K8sGPT analiza un recurso a la vez. TitanOps correlaciona señales de health + security + performance + deployments simultáneamente.
4. **No depende de LLMs cloud en el hot path** — K8sGPT necesita OpenAI/Bedrock/Gemini. TitanOps usa ONNX local para decisiones en tiempo real.
5. **Proactivo vs Reactivo** — K8sGPT se ejecuta cuando alguien lo pide. TitanOps actúa antes de que nadie se entere.

---

## Qué Podemos Aprender de K8sGPT

### 1. Custom Analyzers extensibles

K8sGPT permite registrar `custom_analyzers` — servicios externos que proveen análisis. Patrón interesante:

```yaml
# En k8sgpt config
custom_analyzers:
  - name: titanops-analyzer
    connection:
      url: titanops-gateway
      port: 8080
```

TitanOps podría exponerse como un custom analyzer de K8sGPT, dando acceso a sus correlaciones y acciones autónomas desde K8sGPT.

### 2. Operator pattern con CRDs

K8sGPT tiene un operator que reporta resultados como CRDs:

```yaml
apiVersion: core.k8sgpt.ai/v1alpha1
kind: Result
metadata:
  name: pod-analysis-xyz
spec:
  details: "Pod crasheando por OOM..."
```

TitanOps podría hacer lo mismo: `CorrelatedIncident` y `AutonomousAction` como CRDs consultables con `kubectl get incidents`.

### 3. Integración con Prometheus/Alertmanager

El k8sgpt-operator ya se integra con Prometheus para métricas y Grafana para dashboards. Confirma que nuestra estrategia de integraciones va en la dirección correcta.

### 4. CNCF Sandbox como meta a futuro

Una vez que TitanOps tenga usuarios reales y una comunidad mínima, aplicar a CNCF Sandbox da visibilidad exponencial. K8sGPT lo logró y pasó de proyecto pequeño a conocido.

---

## Estrategia de Integración

### Integración propuesta: TitanOps como custom analyzer de K8sGPT

**Qué:** Un servicio gRPC/HTTP que expone las correlaciones y acciones de TitanOps en el formato que K8sGPT espera para custom analyzers.

**Por qué:** Acceso a 9k+ usuarios de K8sGPT sin esfuerzo. Cuando alguien ejecuta `k8sgpt analyze --custom-analysis`, ve los insights de TitanOps junto con el diagnóstico normal.

**Cómo funciona:**

```
Operador ejecuta: k8sgpt analyze --custom-analysis --explain
                           ↓
K8sGPT llama a TitanOps custom analyzer (HTTP/gRPC)
                           ↓
TitanOps responde: "Incidente correlado: heartbeat degraded + 
                    honeytoken accessed en worker-03. Confidence 92%.
                    Acción tomada: pod isolation."
                           ↓
K8sGPT enriquece con LLM y presenta al usuario
```

**Formato de respuesta del analyzer:**

```json
{
  "results": [
    {
      "kind": "CorrelatedIncident",
      "name": "inc-2026-0719-001",
      "error": [
        {
          "text": "Cross-module correlation: earthworm reported heartbeat_degraded and ebeecontrol reported honeytoken_accessed on node worker-03 within 45s. Confidence: 92%. Action taken: pod_isolation.",
          "sensitive": []
        }
      ],
      "details": "Modules: earthworm, ebeecontrol. Matched: node. Action: isolate_pod. Status: contained.",
      "parent_object": "Node/worker-03"
    }
  ]
}
```

### Integración propuesta: K8sGPT findings como fuente de eventos para TitanOps

**Qué:** Un adaptador que consume los resultados de K8sGPT (via su gRPC serve mode o CRDs del operator) y los convierte en `export.Event` para el correlation engine.

**Por qué:** K8sGPT detecta problemas a nivel de API (pods, services, deployments). Esa info complementa las señales eBPF de TitanOps. Más señales = mejor correlación.

**Cómo funciona:**

```
K8sGPT operator detecta: "PVC pending en namespace production"
                           ↓
Adaptador convierte a export.Event:
  module: "k8sgpt"
  event_type: "pvc_pending"
  namespace: "production"
  severity: "medium"
                           ↓
TitanOps correlation engine lo correlaciona con:
  earthworm: "disk_io anomaly en worker-03"
                           ↓
Incidente correlado: "Disk I/O degraded + PVC pending = possible storage issue"
```

---

## Resumen de decisiones

| Decisión | Razón |
|---|---|
| No competir con K8sGPT | Diferentes capas del problema. Ellos explican, nosotros actuamos. |
| Posicionarnos como complemento | "Usa K8sGPT para entender. Usa TitanOps para automatizar." |
| Implementar custom analyzer adapter | Canal de distribución gratuito hacia 9k+ usuarios |
| Implementar K8sGPT findings como fuente | Más señales para el correlation engine |
| Enfatizar eBPF + autonomous action | Son los diferenciadores que K8sGPT no tiene ni tendrá pronto |
| No aplicar a CNCF aún | Primero producto sólido + usuarios reales. CNCF después. |
