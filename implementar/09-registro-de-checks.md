# 09 · Registro de checks

| Campo | Valor |
| --- | --- |
| Depende de | 02, 03, 05, 08 |
| Habilita | 10, 11, 12, 13, 18 |
| Costo | Medio |
| Valor | Alto |
| Tipo | Formato / Binario |

## Contexto

Una decisión podía nombrar una herramienta en prosa, pero nada decía de forma legible por máquina qué
comando correr, sobre qué parte del store, ni con qué configuración. Esta tarea agrega ese mapa,
`checks.json`, que indexa **herramientas** (no tests puntuales): cada una se instala y configura una vez
y cubre muchos records a la vez.

## Qué se construyó

- **`checks.json`**, misma forma que los otros registros, con `id`, `command` y `justified-by`
  obligatorios, y `enforces`, `config`, `needs` opcionales.
- **`check add` / `check remove`** (sin `check edit`: nada más en el store referencia un check por `id`,
  así que remove+add es tan barato como un edit).
  - `--justified-by` tiene que ser una decisión existente (nunca una spec).
  - `--enforces` acepta un record puntual o una **coordenada de grupo** (`decisions/api/structure`), con
    el mismo mecanismo de recorrido que `validate --for`; un check sin `--enforces` es un check de
    "higiene", aceptado pero con menos peso que uno que sí hace cumplir algo.
  - `--config` (repetible) exige que cada archivo ya exista en el repositorio.
  - `--needs` nombra un sandbox ya declarado en `sandbox.json` (tarea 11); solo registra la dependencia,
    no levanta nada.
- **Sin campo de etapa**: la forma del `command` ya lo dice — si lleva `{files}`, corre solo sobre los
  archivos tocados (`on-change`); si no, corre sobre todo el proyecto (`pre-merge`).
- **El vínculo lo declara el check, no el record**: la decisión queda como prosa pura; la pregunta inversa
  ("¿qué checks hacen cumplir esto?") la calcula el binario.
- **Validaciones** en `validate`, re-chequeadas en cada corrida (no solo al agregar): `check-duplicate`,
  `check-without-command`, `check-justified-by-missing/not-found/not-decision`,
  `check-enforces-not-found/empty-group`, `check-config-not-found`, `check-needs-not-found`,
  `checks-undeclared`, `checks-unreadable`.

## Decisiones

- **Una entrada por herramienta, nunca por record**: listar record por record duplicaría lo que ya dice la
  configuración de la herramienta y se rompería al agregar cada record nuevo.
- **Solo comandos determinísticos**: nada estadístico (carga, SLOs) entra acá — eso es `VERIFIED-IN`
  (tarea 04) o un sandbox (tarea 11).
- **Checks de higiene pesan menos**: uno sin `enforces` no hace cumplir ningún record; contarlo igual que
  uno que sí lo hace premiaría optimizar la métrica en vez de cumplir lo definido.
- **El binario no corre nada acá**: solo indexa y arma los comandos. Correrlos quedó para la tarea 13b.

## Fuera de alcance

- Que el binario corra los checks: correrlos pasó a la tarea 13b (`check run`).
- Un campo de etapa explícito: descartado a favor de que `{files}` ya diga lo necesario.
- Checks estadísticos o que requieren producción real: fuera de `checks.json` por definición.

## Criterios de aceptación

- [x] `check add` y `check remove` existen, escriben `checks.json` con el campo `hash`, y `remove`
      funciona sin dejar referencias rotas.
- [x] `validate` reporta cada situación de la tabla de validaciones, con un test por cada una.
- [x] La documentación explica la regla de `{files}` / sin `{files}` y por qué no hay campo de etapa.
- [x] Existe un decision record que documenta la reapertura de las reglas verificables en el formato.
- [x] La relación `enforces` → grupo del store se resuelve con el mismo mecanismo de coordenadas que ya
      usa `validate --for`.

## Commits

`ca7e0d5` `2ea9046`

## Escenario con IA

`e2e-ai/scenarios/09-checks.md` — PASS (2026-10-14, checked 7/7, judged 6/6, run
`20261014T160803Z-09-checks`, Claude Code 2.1.296 como orlab).
