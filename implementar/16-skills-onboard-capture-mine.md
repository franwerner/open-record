# 16 · Actualizar `openrecord-onboard`, `openrecord-capture` y `openrecord-mine`

| Campo | Valor |
| --- | --- |
| Depende de | 06, 07, 08 |
| Habilita | 17, 18 |
| Costo | Bajo |
| Valor | Medio |
| Tipo | Skill |

## Contexto

Con actores, el catálogo de verificación y la revisión de concerns ya construidos, tres de las siete
skills existentes quedaban desactualizadas: `openrecord-bootstrap` no seguía la convención de verbo del
resto, y ni `capture` ni `mine` preguntaban por el origen de una decisión (qué spec la motiva) ni recorrían
el catálogo nuevo.

## Qué se construyó

- **Renombre**: `skills/openrecord-bootstrap/` → `skills/openrecord-onboard/`, con `name`/`description`
  actualizados y sin referencias viejas en `capture`, `mine` ni `docs/README.md`.
- **`openrecord-onboard`, orden de entrevista**: componentes → **actores** → comportamiento (`flow`,
  `process`, también `rule` y `lifecycle`) → decisiones (recorriendo el catálogo de concerns y
  preguntando "¿esta decisión existe por alguna de estas specs?" para llenar `--serves`) → calidades.
  Después de cada record, sigue preguntando "¿cómo se verifica?" (check, sandbox o `VERIFIED-IN`), como ya
  hacía antes de esta tarea. El orden se invirtió respecto de antes (decisiones antes que comportamiento)
  porque `--serves` se llena mejor cuando la spec que sirve ya existe, en vez de adivinarla "a punto de
  escribirse".
- **`openrecord component edit --title/--description`**: agregado durante el escenario con IA — no había
  forma de cambiar la descripción de un componente después de `component add` sin editar `INDEX.md` a
  mano; `onboard` pasó a usar este comando.
- **`openrecord-capture` y `openrecord-mine`, pregunta de origen**: antes de escribir una decisión, cada
  skill resuelve (`capture`) o propone (`mine`) a qué spec sirve, corriendo `openrecord impact --files
  <lo que tocó>` para encontrar qué specs gobiernan esos archivos. En `capture` el agente lo resuelve él
  mismo sin preguntar (hizo el trabajo: la skill ya no pide confirmación para nada que escribe); en
  `mine` se propone con la evidencia y la persona la confirma, igual que el resto de lo que esa skill
  reconstruye. Si en el mismo cierre se escribe spec y decisión, la spec va primero. Regla explícita en
  las dos skills: **"never invent a why"** — un `--serves` falso es peor que ninguno. El reporte final de
  `capture` dice a qué spec sirve cada decisión que escribió.
- **Controles que leen `serves` aceptan la forma en una línea** (`serves: [a]`), que es como la escribe el
  binario.

## Decisiones

- **El renombre se adoptó por el par `onboard`/`define`** ("entrevista amplia" / "entrevista acotada"),
  no porque "onboard" sea un verbo puro como el resto de las skills.
- **La pregunta de origen terminó llegando a las tres skills en esta misma tarea**, no solo a `capture`
  como se había previsto al principio: una vez resuelta para `capture`, extenderla a `mine` fue el mismo
  mecanismo (`impact --files`) con la única diferencia de quién confirma.
- **Quién confirma sigue el mismo criterio que ya tenían las dos skills**: en `capture`, nadie — la skill
  resuelve y escribe; en `mine`, siempre la persona, porque el razonamiento salió de leer código, no de
  una conversación, y la skill nunca puede recuperar el porqué original.

## Fuera de alcance

- Cambiar `openrecord-consult`, `openrecord-reconcile` o `openrecord-audit`: ninguna pieza de esta ronda
  las afecta.
- Migrar automáticamente una instalación existente de `skills/openrecord-bootstrap/`: se resuelve con una
  nueva emisión (`openrecord skills --emit`), que ya sabe limpiar lo que no se distribuye más.

## Criterios de aceptación

- [x] `skills/openrecord-bootstrap/` pasa a `skills/openrecord-onboard/`, con `name` y `description`
      actualizados.
- [x] Ninguna referencia a `openrecord-bootstrap` queda en `capture`, `mine` o `docs/README.md`.
- [x] `openrecord-onboard` pregunta actores antes de la primera spec de flujo o proceso, y recorre el
      catálogo de verificación y los temas de `docs/concerns.md` como túnel de preguntas.
- [x] `openrecord-capture` y `openrecord-mine` preguntan (o resuelven) origen y verificación antes de
      proponer un record, manteniendo quién confirma cada una según ya estaba definido.
- [x] `openrecord skills --emit` sobre una instalación existente reporta la skill vieja como retirada y la
      nueva como creada.

## Commits

`ed4e5c0` `9949ec3` `0457d5d` `19e1968` `aaddd8d` `20f1f4b` `0eaee05`

## Escenario con IA

- `e2e-ai/scenarios/16-onboard.md` — PASS (2026-10-21, checked 7/7, judged 3/3, run
  `20261021T074413Z-16-onboard`).
- `e2e-ai/scenarios/16-capture.md` — PASS (2026-10-21, checked 4/4, judged 2/2, run
  `20261021T110938Z-16-capture`).
- `e2e-ai/scenarios/16-mine.md` — PASS (2026-10-21, checked 4/4, judged 3/3, run
  `20261021T115227Z-16-mine`).

Claude Code 2.1.296 como orlab en los tres.
