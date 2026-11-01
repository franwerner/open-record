# 17 · Definir puntual, dentro de `capture` (ya no skill `openrecord-define`)

| Campo | Valor |
| --- | --- |
| Depende de | 12, 15, 16 |
| Habilita | 18 |
| Costo | Medio |
| Valor | Alto |
| Tipo | Skill |

## Contexto

La mayoría de las decisiones de un proyecto maduro aparecen una por una: una decisión nueva, una spec,
un actor que faltaba, o un hueco que dejó un run. `openrecord-onboard` recorre todo el sistema y es
desproporcionado para eso. Se decidió en la 16 que no hay skill `define`: capturar y definir terminan
igual (un record escrito por el binario) y lo que cambia es **de dónde sale el porqué**, regla que
`capture` ya tenía.

## Qué se construyó

- **`openrecord-capture` con dos situaciones** (`skills/openrecord-capture/SKILL.md`):
  1. **Terminé algo** (sin cambios): el agente resuelve, escribe y reporta; nadie confirma.
  2. **Quiero definir algo puntual**: el porqué viene de la persona o de un hueco. Si dijo qué pero no
     por qué, se pregunta. Se propone **solo título y una línea**; nunca el cuerpo ni un resumen de él
     ("un resumen del cuerpo es el cuerpo"); solo se puede agregar lo que se dejó afuera y por qué. El sí
     escribe directo como `accepted`. Igual para `actor add`, `component add`, `check add`,
     `sandbox add`. Tabla de qué preguntar para cada tipo: actor, decisión, spec, componente, hueco,
     cómo se verifica algo existente. Se agregó la mecánica de `component add`.
- La tabla "quién confirma" ganó la fila de esta situación. La `description` dispara en los dos casos.
- **Pasos de coherencia**, en las dos situaciones: vecindario (`impact`/`impact --files`), origen
  (`--serves`), dependientes (`dependents`/`served_by` de la respuesta), catálogo de verificación y
  **`reconcile` sobre el borrador, antes de escribir** (nuevo también para la situación 1). En la
  situación 1 un choque dispara el bloque STOPPED existente; en la 2 se muestra antes de proponer y no se
  escribe nada hasta que la persona decida.
- **Huecos**: sin nada nombrado, `openrecord run gaps` (nunca leer `.openrecord/runs/`). Por tipo:
  - `missing`: record nuevo con la persona, y `run gap resolve --record` con él;
  - `pending`: el record lo escribió otro (p. ej. `mine`), así que **se muestra el cuerpo** — la única
    excepción — y se acepta, cambia o descarta; aceptar es `record edit --status accepted`;
  - `contradicts`: las dos partes, decide la persona, se edita el que corresponde.
  `gap resolve` devuelve las tareas bloqueadas a `pending` **en el mismo run** (salvo otro hueco abierto)
  y no escribe en `compliance`.
- **`openrecord-reconcile`**: párrafo para comparar un borrador (`map`, `component owners`, `search
  --context` funcionan igual; `impact` corre sobre los `--serves`/`--depends-on` que va a declarar). Su
  `description` dispara también "antes de escribir un record".
- `docs/README.md`: la línea de `openrecord-capture` cubre las dos situaciones.

## Decisiones tomadas al definirla

- **No se revisa si el código ya cumple** (ni `audit` ni `check run`) al definir: el record nuevo queda
  `unknown` en `compliance` y llevarlo al código es de la 18. Costo aceptado: la persona no se entera en
  el momento de que el código actual ya lo incumple.
- `reconcile` en las dos situaciones y antes de escribir, para no dejar nunca un `accepted` que
  contradice a otro.
- El criterio original "las tareas bloqueadas se retoman en la siguiente ejecución vía `compliance`"
  contradecía lo construido en la 15; la skill describe lo que hace el binario.

## Pruebas

- Deterministas: no hay código nuevo; `scripts/check.sh fast` pasa.
- Con IA (`e2e-ai/scenarios/`): `17-define`, `17-gaps`, `17-conflict` y la regresión `16-capture`, todas
  PASS el 2026-10-23. `17-define` falló la primera vez (el agente resumió el cuerpo en viñetas después
  de la propuesta); se corrigió la skill.

## Commits

`bf62f3c` `07b03db`
