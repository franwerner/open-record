# 11 · Sandbox

| Campo | Valor |
| --- | --- |
| Depende de | 04, 09 |
| Habilita | 13, 15, 18 |
| Costo | Medio |
| Valor | Medio |
| Tipo | Formato / Binario |

## Contexto

Un test puede pasar mientras la salida real del sistema muestra algo mal (un error tragado, reintentos de
más, consultas repetidas): nada de eso lo atrapa un código de salida. Esta tarea define `sandbox.json`,
instrucciones de cómo levantar un entorno para observarlo — nunca un orquestador propio.

## Qué se construyó

- **`sandbox.json`**, con `description` obligatoria y `components`, `setup`, `start`, `ready` (exactamente
  uno de `http` o `log`, con `timeout` entero de segundos, default `60`), `exercise`, `approximates`,
  `observes`, `logs` (`source` + `format` `json`/`text`, default `text`), `stop`.
- **`sandbox add` / `sandbox edit` / `sandbox remove`**: `add` exige `description`, exactamente una
  condición de `ready`, `logs-source` y `stop`, y al menos uno de `setup`/`start`; los defaults
  (`ready.timeout`, `logs.format`) se escriben explícitos en el archivo, nunca quedan implícitos. `edit`
  reemplaza lo que se le pasa (una lista repetible reemplaza entera, nunca agrega); cambiar de
  `--ready-http` a `--ready-log` limpia el lado contrario. `remove` se niega mientras algún `needs` de
  `checks.json` lo siga nombrando.
- **`--approximates`** (repetible): vincula el sandbox a un ancla exacta de una spec `quality`
  (`specs/quality/<nombre>.md#<ancla>`, ancla obligatoria — a diferencia de `qualifies`, un sandbox
  aproxima un escenario a la vez). Detecta regresión contra la corrida anterior en la misma máquina; nunca
  certifica el umbral exacto.
- **`--observes`** (repetible): decisiones contra las que se juzga la salida — para una regla que solo se
  nota con volumen real; si se puede comprobar de forma determinística, va a `checks.json` en vez de acá.
- **`sandbox run`** quedó finalmente implementado en la tarea 13b, no en esta: esta tarea entregó el
  registro (`sandbox add`/`edit`/`remove`) y la conexión declarativa con `checks.json` (`needs`); el
  binario de verdad levantando, ejerciendo y apagando un sandbox es 13b.

## Decisiones

- **Nombre `sandbox`, no `observability` ni `runtime`**: los dos ya son nombres de concern en
  `docs/concerns.md`; una palabra no puede significar dos cosas en el vocabulario que lee un agente (misma
  regla que disolvió el concern `quality`).
- **`exercise` es deliberadamente no un check**: no importa si "pasa", importa la salida que produce;
  conviene que sea un comando fijo para que un error nuevo se atribuya al cambio, no a un recorrido
  distinto.
- **Puertos y variables de entorno quedan del lado del proyecto**: el sandbox no los maneja; si el
  checkout y un worktree de ejecución (tarea 15/20) levantan lo mismo a la vez, el choque se reporta como
  error de entorno, no de la tarea.

## Fuera de alcance / diferido

- Manejo de puertos o variables de entorno: del lado del proyecto.
- Certificar un umbral de calidad: un sandbox aproxima y detecta regresiones, nunca certifica.
- Inspección visual de UI, señal gruesa de memoria durante `exercise`, reproducir errores antes de
  arreglarlos: posibles mejoras futuras, sin tarea asignada.
- Levantar, ejercer y apagar de verdad un sandbox (`sandbox run`): tarea 13b.

## Criterios de aceptación

- [x] `sandbox add` y `sandbox remove` existen, escriben `sandbox.json` con `hash`, y validan sus campos
      obligatorios.
- [x] `validate` reporta cada situación de la tabla de validaciones, con un test por cada una.
- [x] `checks.json` acepta `needs` apuntando a un sandbox existente, con un test que cubre la referencia
      rota.
- [x] La documentación explica la diferencia entre `approximates` y `observes`, con un ejemplo de cada
      uno.
- [x] Existe un decision record que documenta por qué este registro se llama `sandbox`.

## Commits

`e1db210` `9ac36de`

## Escenario con IA

`e2e-ai/scenarios/11-sandbox.md` — PASS (2026-10-16, checked 7/7, judged 4/4, run
`20261016T165125Z-11-sandbox`, Claude Code 2.1.296 como orlab).
