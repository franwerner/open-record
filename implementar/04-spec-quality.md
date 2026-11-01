# 04 · Spec `quality`

| Campo | Valor |
| --- | --- |
| Depende de | 01 |
| Habilita | 07, 08, 09, 11, 12, 13 |
| Costo | Medio |
| Valor | Alto |
| Tipo | Formato |

## Contexto

Los cuatro tipos de spec existentes (`flow`, `rule`, `lifecycle`, `process`) describen comportamiento
funcional, respondible con sí/no en cada caso. Nada tenía forma pensada para un atributo medible ("p95 <
10 s"), que terminaba mezclado dentro de una spec funcional o disfrazado de decisión. Esta tarea agrega
un quinto tipo, `quality`, exclusivamente para eso.

## Qué se construyó

- **`quality` como quinto tipo de spec**, en `specs/quality/`, con frontmatter `components` (igual que
  cualquier spec) y `qualifies` opcional.
- **`qualifies`**: una o varias specs funcionales, enteras o por ancla (`specs/flow/upload-file.md#
  successful-upload`), que esta calidad caracteriza. Es del archivo entero, no de cada escenario — dos
  escenarios que califican comportamientos distintos van en dos specs separadas. Se omite cuando la
  calidad es una propiedad global del sistema. Validado con sus propios códigos: `qualifies-not-found`,
  `qualifies-anchor-not-found`, `qualifies-not-functional` (el destino es otra `quality`),
  `qualifies-not-quality` (el campo está en una spec que no es `quality`).
- **Secciones**: solo el núcleo que ya tiene toda spec (`## Purpose`, `## Scenarios`); `quality` no agrega
  secciones propias. Cualquier otra sección es `unexpected-section`.
- **Forma del escenario**: `### Scenario: <nombre>` como cualquier spec (sin identificador `[Qn]` propio —
  ver tarea 01: se consideró y se descartó para compartir un único espacio de anclas), GIVEN/WHEN/THEN
  más **`MEASURE`** obligatorio.
- **`VERIFIED-IN`**: línea opcional y repetible después de `MEASURE`, solo en `quality`, con un valor fijo
  (`pre-release`, `production`, `manual`) y texto libre de dónde mirar. Es informativa: el binario nunca
  lee ni sigue esa fuente. `verified-in-invalid` y `verified-in-not-quality` la validan.
- **Tres fuentes, nunca escritas en la spec**, que limpian la advertencia `quality-unverified`: un check
  cuyo `enforces` cubre la spec entera o un grupo que la contiene (tarea 09), un sandbox cuyo
  `approximates` nombra ese ancla exacto (tarea 11), o la propia `VERIFIED-IN`.
- **`openrecord verification <atributo>`** (tarea 08) dice qué necesita contener un escenario de un
  atributo dado para ser verificable, y qué valor de `VERIFIED-IN` suele tomar.

## Decisiones

- **Prueba funcional/calidad**: si el resultado es sí o no en cada caso concreto, es funcional; si
  necesita un percentil, una carga o un entorno para medirse, es `quality`.
- **Sin `[Qn]`**: una spec `quality` ancla exactamente igual que cualquier otra (tarea 01), para no abrir
  una segunda regla de anclaje en el store.
- **`qualifies` es una relación distinta de `serves`** (tarea 05), con nombre propio a propósito, para que
  un reporte de vecindario no confunda "esto caracteriza ese comportamiento" con "esto sirve a esa spec".
- **Sin lector automático de `VERIFIED-IN`**: leer una fuente externa con un agente sin supervisión pone
  al alcance credenciales que hoy quedan fuera del sandbox; se evaluó como pieza futura opcional y quedó
  fuera.

## Fuera de alcance / diferido

- Un lector automático que vaya a buscar el resultado que declara `VERIFIED-IN`: evaluado y descartado por
  ahora, sin tarea asignada.
- Verificar que el `MEASURE` de un escenario sea alcanzable o razonable: es contenido de la spec, no algo
  que el binario pueda juzgar.

## Criterios de aceptación

- [x] `record write` y `record edit` aceptan `type: quality` con su frontmatter y secciones propias.
- [x] `qualifies` se valida contra specs y anclas existentes, con tests que cubren una referencia rota y
      una referencia a un tipo de spec incorrecto.
- [x] El parser de escenarios reconoce `quality` con `MEASURE` obligatorio; `VERIFIED-IN` se reconoce
      como línea opcional y se valida contra los tres valores permitidos.
- [x] `validate` emite `quality-unverified` cuando corresponde, con tests que la disparan y que confirman
      que desaparece al declarar un check, un `approximates` o un `VERIFIED-IN`.
- [x] `docs/capability-spec.md` documenta el quinto tipo con su frontmatter, su forma de escenario y la
      tabla de `VERIFIED-IN`.
- [x] Existe un decision record que documenta la apertura de este tipo en el formato.

## Commits

`027902c` `ca6fbba`

## Escenario con IA

`e2e-ai/scenarios/04-quality.md` — PASS (2026-10-13, checked 8/8, judged 4/4, run
`20261013T231246Z-04-quality`, Claude Code 2.1.296 como orlab).
