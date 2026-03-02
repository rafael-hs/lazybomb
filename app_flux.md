# Arquitetura do lazybomb

## Visão Geral

```
┌─────────────────────────────────────────────────────────┐
│                        main.go                          │
│              entrypoint minimalista                     │
└───────────────────────────┬─────────────────────────────┘
                            │ chama tui.Run()
┌───────────────────────────▼─────────────────────────────┐
│                   internal/tui/                         │
│         Padrão Elm: Model → Update → View               │
│                                                         │
│  tui.go       model.go    update.go   view.go           │
│  messages.go  keys.go                                   │
└──────┬─────────────────────────────┬────────────────────┘
       │ runner.Start()              │ config.LoadProfiles()
       │ runner.Stop()               │ config.SaveProfile()
       │ runner.Snapshot()           │ config.DeleteProfile()
       ▼                             ▼
┌──────────────────┐    ┌────────────────────────────────┐
│ internal/runner/ │    │      internal/config/          │
│                  │    │                                │
│  runner.go       │    │  profiles.go                   │
│  metrics.go      │    │  ~/.config/lazybomb/            │
│                  │    │    profiles.json               │
│  HTTP worker pool│    │                                │
│  + Aggregator    │    │  CRUD de perfis                │
└──────────────────┘    └────────────────────────────────┘
```

---

## O Padrão Elm (TEA — The Elm Architecture)

Tudo gira em torno de um ciclo infinito e determinístico:

```
evento do terminal / goroutine
           ↓
    Update(msg) → novo Model + Cmd
           ↓
    View(model) → string ANSI renderizada no terminal
           ↓
    (próximo evento...)
```

**Três propriedades fundamentais:**
- `Model` é o único lugar onde existe estado
- `Update` é a única forma de mudar o estado
- `View` é uma função pura — dado o mesmo `Model`, sempre produz a mesma string

---

## `main.go` — Entrypoint

```go
func main() {
    // redireciona logs para /tmp/lazybomb.log
    // (não polui o terminal enquanto a TUI roda)
    if f, err := os.OpenFile("/tmp/lazybomb.log", ...); err == nil {
        log.SetOutput(f)
    }
    if err := tui.Run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

Nenhuma lógica. Apenas configura logging e delega para `tui.Run()`.

---

## `internal/tui/tui.go` — Inicialização

```go
var currentProgram *tea.Program   // ← ponto crítico da arquitetura

func Run() error {
    m := initialModel()
    p := tea.NewProgram(m,
        tea.WithAltScreen(),        // tela alternativa (não mexe no histórico)
        tea.WithMouseCellMotion(),  // suporte a mouse
    )
    currentProgram = p
    _, err := p.Run()
    return err
}
```

`currentProgram` é a única variável global do projeto. Ela existe para resolver um problema fundamental: goroutines do runner rodam fora do event loop do Bubble Tea, mas precisam injetar mensagens nele. O `p.Send(msg)` é thread-safe e é a ponte entre os dois mundos.

---

## `internal/tui/model.go` — O Estado

O `Model` é a fonte única de verdade de toda a aplicação:

```
Model
├── width, height          — dimensões do terminal (recebidas via WindowSizeMsg)
│
├── ── Navegação ──────────────────────────────────────────
├── activePanel            — qual painel está visível (Config/Metrics/Profiles)
├── activeField            — qual campo está focado dentro do Config
│
├── ── Painel Config ──────────────────────────────────────
├── inputs [fieldCount]    — array de 15 textinput.Model (um por campo)
├── authKind               — tipo de auth selecionado (None/Bearer/Basic/APIKey)
│
├── ── Painel Profiles ────────────────────────────────────
├── profiles []Profile     — lista de perfis carregados do disco
├── profileCursor          — linha selecionada na lista
├── profileErr             — mensagem de erro específica do painel
│
├── ── Estado do Runner ───────────────────────────────────
├── runner *runner.Runner  — instância do engine de load test
├── running bool           — teste em andamento
├── done bool              — teste concluído
├── stopped bool           — teste parado pelo usuário (vs. concluído naturalmente)
├── lastSnap runner.Snapshot — último snapshot de métricas recebido
│
└── ── Status Bar ─────────────────────────────────────────
    ├── statusMsg          — mensagem de status (ex: "Running...", "Done — 200 req")
    ├── err                — mensagem de erro na barra inferior
    └── showHelp           — toggle do overlay de ajuda
```

### Sistema de Campos (configField)

Há 15 campos definidos como enum `configField`. Eles definem a **ordem de navegação** com ↑↓:

```
fieldURL(0) → fieldMethod(1) → fieldHeaders(2) → fieldBody(3)
→ fieldRequests(4) → fieldConcurrency(5) → fieldDuration(6)
→ fieldRateLimit(7) → fieldTimeout(8)
→ fieldAuthType(9)*          ← campo VIRTUAL
→ fieldAuthToken(10)         ← só visível se authKind == Bearer
→ fieldAuthUser(11)          ← só visível se authKind == Basic
→ fieldAuthPass(12)          ← só visível se authKind == Basic
→ fieldAuthKeyName(13)       ← só visível se authKind == APIKey
→ fieldAuthKeyValue(14)      ← só visível se authKind == APIKey
→ (volta para fieldURL)
```

`fieldAuthType` é um **campo virtual**: existe no enum para participar da navegação, mas não tem `textinput.Model`. É um seletor visual operado com ←/→. Isso exige guards em 4 lugares: `nextField()`, `updateFocusedInput()`, `textinputBlink()` e `handleConfigKey()`.

`isFieldVisible(f)` determina quais campos de auth aparecem/são navegáveis com base no `authKind` atual.

---

## `internal/tui/messages.go` — Mensagens

São os 3 tipos de eventos que chegam de fora do loop de eventos (das goroutines do runner):

```go
tickMsg  { snap runner.Snapshot }        // a cada 200ms: métricas ao vivo
doneMsg  { snap runner.Snapshot         // teste finalizado
           stopped bool }               //   stopped=true se usuário parou
errMsg   { err error }                  // erro ao iniciar o teste
```

Esses tipos são o contrato de comunicação entre o runner e a TUI.

---

## `internal/tui/keys.go` — Keybindings

Todos os atalhos em um lugar, usando o sistema declarativo do `bubbles/key`:

```go
var keys = keyMap{
    Tab:      key.NewBinding(key.WithKeys("tab")),
    ShiftTab: key.NewBinding(key.WithKeys("shift+tab")),
    Enter:    key.NewBinding(key.WithKeys("enter", "ctrl+r")),
    Stop:     key.NewBinding(key.WithKeys("esc")),
    Save:     key.NewBinding(key.WithKeys("ctrl+s")),
    Load:     key.NewBinding(key.WithKeys("ctrl+l")),
    Delete:   key.NewBinding(key.WithKeys("ctrl+d")),
    Up:       key.NewBinding(key.WithKeys("up", "k")),    // vim keys
    Down:     key.NewBinding(key.WithKeys("down", "j")),
    Left:     key.NewBinding(key.WithKeys("left", "h")),
    Right:    key.NewBinding(key.WithKeys("right", "l")),
    Quit:     key.NewBinding(key.WithKeys("ctrl+c", "q")),
    Help:     key.NewBinding(key.WithKeys("?")),
}
```

Usado como `key.Matches(msg, keys.Enter)`. Centralizar aqui evita strings espalhadas e facilita gerar o overlay de help automaticamente.

---

## `internal/tui/update.go` — Lógica de Transição

É onde 100% da lógica de negócio da UI vive. A função `Update` recebe qualquer `tea.Msg` e retorna `(Model, Cmd)`:

### Árvore de dispatch completa

```
Update(msg)
├── tea.WindowSizeMsg  → m.width, m.height = msg.Width, msg.Height
│
├── tickMsg            → m.lastSnap = msg.snap
│                        (Bubble Tea redesenha automaticamente)
│
├── doneMsg            → m.lastSnap = msg.snap
│                        m.running = false
│                        m.done = true
│                        m.stopped = msg.stopped
│                        m.statusMsg = "Done — N requests in Xs"
│
├── errMsg             → m.err = msg.err.Error()
│                        m.running = false
│
└── tea.KeyMsg         → handleKey(msg)
      ├── keys.Quit    → runner.Stop() + tea.Quit
      ├── keys.Help    → m.showHelp = !m.showHelp
      │
      └── switch activePanel:
            ├── panelConfig   → handleConfigKey(msg)
            │     ├── [se fieldAuthType focado]
            │     │     keys.Left  → authKind cicla para trás
            │     │     keys.Right → authKind cicla para frente
            │     │
            │     ├── keys.Enter  → startTest()
            │     ├── keys.Stop   → runner.Stop()
            │     ├── keys.Save   → saveProfile()
            │     ├── keys.Load   → activePanel = panelProfiles
            │     ├── keys.Tab    → cyclePanel(+1)
            │     ├── keys.ShiftTab → cyclePanel(-1)
            │     ├── keys.Down   → nextField(+1) + textinputBlink()
            │     ├── keys.Up     → nextField(-1) + textinputBlink()
            │     └── (default)  → updateFocusedInput(msg)
            │                       (repassa para o textinput ativo)
            │
            ├── panelProfiles → handleProfilesKey(msg)
            │     ├── keys.Up    → profileCursor--
            │     ├── keys.Down  → profileCursor++
            │     ├── keys.Enter → loadProfile() + activePanel = panelConfig
            │     ├── keys.Delete → deleteProfile()
            │     ├── keys.Stop  → activePanel = panelConfig
            │     ├── keys.Tab   → cyclePanel(+1)
            │     └── keys.ShiftTab → cyclePanel(-1)
            │
            └── panelMetrics → handleMetricsKey(msg)
                  ├── keys.Stop  → runner.Stop()
                  ├── keys.Tab   → cyclePanel(+1)
                  └── keys.ShiftTab → cyclePanel(-1)
```

### Funções auxiliares chave

**`startTest()`** — orquestra o início de um teste:
```
1. buildConfig()   → lê todos os inputs, parseia headers, injeta auth
2. runner.Start()  → lança goroutines, registra callbacks
3. activePanel = panelMetrics
```

**`buildConfig()`** — converte estado do Model em `runner.Config`:
- Lê cada `inputs[field].Value()`
- `parseHeaders("Content-Type: json | X-Foo: bar")` → `map[string]string`
- Injeta header de auth sobrescrevendo qualquer `Authorization` manual:
  ```go
  case authBearer: headers["Authorization"] = "Bearer " + token
  case authBasic:  headers["Authorization"] = "Basic " + base64(user:pass)
  case authAPIKey: headers[keyName] = keyValue
  ```

**`nextField(dir)`** — navegação com skip de campos invisíveis:
```
1. Blur campo atual (se não for virtual)
2. next = (current + fieldCount + dir) % fieldCount
3. while !isFieldVisible(next): avança/recua
4. m.activeField = next
5. Focus no novo campo (se não for virtual)
```

**`parseHeaders(raw)`** — separador `|` (não `,`, pois vírgulas aparecem em valores de headers como `Accept: text/html, application/json`).

---

## `internal/tui/view.go` — Renderização

Função pura. Recebe `Model`, retorna `string`. Nenhum side-effect, nenhum acesso a estado externo.

### Layout geral

```
┌─ Tabs ─────────────────────────────────────────────┐
│  Config    Metrics    Profiles                      │  ← tabsView()
│────────────────────────────────────────────────────│
│                                                    │
│  [conteúdo do painel ativo]                        │  ← configView() / metricsView() / profilesView()
│                                                    │
├─ Status bar ───────────────────────────────────────┤
│  mensagem de status        tab · ? help · q quit   │  ← statusBarView()
└────────────────────────────────────────────────────┘
```

### Painel Config

```
Request Configuration

URL          [https://example.com                ]
Method       [GET     ]
Headers      [Content-Type: application/json | …]
Body         [                                  ]
Requests     [200       ]
Concurrency  [10        ]
Duration     [          ]
Rate limit   [0         ]
Timeout (s)  [20        ]

── Auth ─────────────────────────────────────────────
Auth         [ ● Bearer  ○ None  ○ Basic  ○ API Key ]  ←/→ change
Token        [eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9…]

↑↓ navigate  •  enter run  •  ctrl+s save  •  ctrl+l load
```

### Painel Metrics (split 50/50)

```
┌─ Live Metrics  ● RUNNING ─────────────────────────────────────┐
│ Elapsed      12.3s    │  Latency histogram                    │
│ Total req    1200     │  10ms ████████████ 450               │
│ Success      1198     │  20ms ██████       180               │
│ Errors       2        │  50ms ███          90                │
│                       │                                       │
│ RPS          97.52    │                                       │
│                       │                                       │
│ Fastest      8.21ms   │                                       │
│ Average      14.3ms   │                                       │
│ Slowest      312ms    │                                       │
│                       │                                       │
│ p50          12.1ms   │                                       │
│ p90          28.4ms   │                                       │
│ p99          89.2ms   │                                       │
│                       │                                       │
│ Status codes          │                                       │
│   200: 1198           │                                       │
│   500: 2              │                                       │
│                       │                                       │
│ Latency/sec           │                                       │
│ ▁▂▃▄▅▃▂▁▂▃           │                                       │
└───────────────────────────────────────────────────────────────┘
```

Sparkline usa caracteres `▁▂▃▄▅▆▇█` normalizados entre o min e max da janela visível.

### Estilos e cores

```go
colorAccent  = "#7C3AED"  // purple  — painéis ativos, destaque
colorSuccess = "#10B981"  // green   — 2xx, p50, fastest
colorWarn    = "#F59E0B"  // amber   — 4xx, p99, RUNNING
colorError   = "#EF4444"  // red     — 5xx, erros, STOPPED
colorMuted   = "#6B7280"  // grey    — labels, hints, bordas inativas
colorBg      = "#1E1E2E"  // dark    — fundo
colorFg      = "#CDD6F4"  // light   — texto principal
```

Painel ativo tem borda `colorAccent`, painéis inativos têm `colorMuted`.

---

## `internal/runner/runner.go` — Engine de Load Test

Completamente desacoplado da TUI. Não importa nada de `internal/tui`.

### Ciclo de vida de um teste

```
runner.Start(cfg, 200ms, onTick, onDone)
         │
         ├─ valida URL
         ├─ cria context com cancel (+ timeout se Duration configurada)
         ├─ buildClient(cfg) → *http.Client configurado
         ├─ aggregator.Start() → registra startTime
         └─ go runLoad(...) ─────────────────────────┐
                                                     │
runLoad():                                           │
  ├─ goroutine ticker (200ms)                        │
  │     loop: select ctx.Done | ticker.C             │
  │           → onTick(aggregator.Snapshot())        │
  │             → p.Send(tickMsg{snap})              │
  │                                                  │
  ├─ [se RateLimit > 0]                              │
  │     rateCh = time.NewTicker(1s / RateLimit).C   │
  │                                                  │
  ├─ sem = make(chan struct{}, Concurrency)           │
  │     // semáforo controla workers simultâneos     │
  │                                                  │
  └─ for i := 0; i < Requests; i++:                 │
        [aguarda rateCh se rate limiting ativo]      │
        [aguarda sem (concurrency gate)]             │
        go doRequest(ctx, cfg, client)               │
             │                                       │
             ├─ buildRequest(cfg)                    │
             ├─ req.WithContext(ctx)                 │
             ├─ start = time.Now()                   │
             ├─ client.Do(req)                       │
             ├─ latency = time.Since(start)          │
             └─ aggregator.Record(latency, code, err)│
                                                     │
  wg.Wait() ← aguarda todos os workers terminarem   │
  onDone(aggregator.Snapshot(), stopped)             │
  → p.Send(doneMsg{snap, stopped})    ◄──────────────┘
```

### Controle de concorrência e cancelamento

- **Semáforo:** `chan struct{}` de capacidade `Concurrency`. Worker adquire ao entrar (`sem <- struct{}{}`), libera ao sair (`<-sem`).
- **Rate limiting:** `time.Ticker` com intervalo `time.Second / RateLimit`. O loop principal aguarda no canal antes de lançar cada request.
- **Cancelamento:** `context.WithCancel`. Ao chamar `runner.Stop()`, o `cancelFn()` é invocado. Todos os goroutines monitoram `ctx.Done()` e terminam graciosamente.
- **Duration:** se configurado, usa `context.WithTimeout` sobre o context base.

---

## `internal/runner/metrics.go` — Agregação

### `Aggregator` — coleta thread-safe

```go
type Aggregator struct {
    mu          sync.Mutex
    startTime   time.Time
    latencies   []float64          // todas as latências em segundos
    statusCodes map[int]int        // contagem por código HTTP
    errors      int
    timeBuckets map[int][]float64  // latências agrupadas por segundo (sparkline)
}
```

`Record(latency, code, isError)` é chamado por múltiplas goroutines simultaneamente — o mutex protege todas as escritas.

### `Snapshot()` — cálculo de métricas

A cada chamada (a cada 200ms):

```
1. Lock mutex
2. Copia latencies → sorted []float64
3. sort.Float64s(sorted)                    ← O(n log n) a cada tick
4. fastest = sorted[0]
5. slowest  = sorted[n-1]
6. average  = sum / n
7. rps      = n / elapsed.Seconds()
8. p50, p75, p90, p99 via percentile()
9. buildHistogram() → 10 buckets entre fastest e slowest
10. latencyOverTime → média por segundo para sparkline
```

**Trade-off:** sort completo a cada 200ms funciona bem para uso típico. Em testes longos com centenas de milhares de requests, isso se tornaria o gargalo — ferramentas como vegeta usam `t-digest` para percentis em O(1) de memória.

### `percentile(sorted, p)`

```go
idx := ceil(p/100.0 * n) - 1
return sorted[idx]
```

### `buildHistogram(sorted, fastest, slowest)`

10 buckets de largura uniforme entre `fastest` e `slowest`. Cada bucket recebe contagem e frequência relativa (`count/total`).

---

## `internal/config/profiles.go` — Persistência

CRUD simples em `~/.config/lazybomb/profiles.json`:

```json
{
  "profiles": [
    {
      "name": "https://api.example.com",
      "url": "https://api.example.com",
      "method": "GET",
      "requests": 200,
      "concurrency": 10,
      "timeout": 20,
      "auth_kind": 1,
      "auth_token": "eyJhbG..."
    }
  ]
}
```

- `LoadProfiles()` — lê do disco, retorna `[]` se arquivo não existe
- `SaveProfile(p)` — upsert por nome (mesmo nome = substituição)
- `DeleteProfile(name)` — filtra e reescreve o arquivo

`auth_kind` como inteiro (0=None, 1=Bearer, 2=Basic, 3=APIKey). Campos de auth com `omitempty` — perfis sem auth não poluem o JSON.

---

## Fluxo Completo: do Input ao Request HTTP e de Volta

```
[usuário presseia Enter no painel Config]
              ↓
handleConfigKey(KeyMsg) → startTest()
              ↓
buildConfig():
  inputs[fieldURL].Value()         → "https://api.example.com"
  inputs[fieldHeaders].Value()     → "Content-Type: json"
  parseHeaders()                   → map[string]string
  authKind == authBearer           → headers["Authorization"] = "Bearer ..."
  → runner.Config{URL, Method, Headers, Body, Requests, Concurrency, ...}
              ↓
runner.Start(cfg, 200ms, onTick, onDone):
  ctx, cancel = context.WithCancel(...)
  buildClient(cfg) → *http.Client
  aggregator.Start() → registra startTime
  go runLoad(...)
              ↓
activePanel = panelMetrics  ← TUI muda para painel de métricas

════════════════ goroutines do runner ════════════════

[goroutine ticker, a cada 200ms]
  aggregator.Snapshot() → Snapshot{Total, RPS, P50, P90, P99, Histogram, ...}
  currentProgram.Send(tickMsg{snap})
              ↓
[Bubble Tea event loop]
  Update(tickMsg) → m.lastSnap = snap
  View(m) → renderiza painel Metrics com métricas atualizadas
              ↓
[terminal exibe métricas ao vivo]

[goroutine de workers]
  for i < Requests:
    sem <- struct{}{}          ← aguarda slot de concorrência
    go doRequest():
      buildRequest(cfg)        → *http.Request com todos os headers
      client.Do(req)           → HTTP request
      aggregator.Record(latência, statusCode, isError)
      <-sem                    ← libera slot

[ao finalizar todos os workers]
  onDone(snap, stopped)
  currentProgram.Send(doneMsg{snap, stopped})
              ↓
Update(doneMsg):
  m.running = false
  m.done = true
  m.statusMsg = "Done — 200 requests in 2.1s"
  View() → ✓ DONE na header do painel Metrics
```

---

## Pontos de Atenção Arquiteturais

| Aspecto | Situação Atual | Impacto |
|---|---|---|
| `currentProgram` global | variável de pacote em `tui.go` | Baixo — confinada ao pacote `tui` |
| Sort completo em `Snapshot()` | `O(n log n)` a cada 200ms | Cresce com o volume de requests |
| `Model` monolítico | todo o estado em uma struct | Adequado para o tamanho atual |
| `fieldAuthType` virtual | guards em 4 lugares | Funcional, mas acoplamento implícito |
| Senhas em perfis | `auth_pass` em JSON plaintext | Risco de segurança se arquivo vazar |
