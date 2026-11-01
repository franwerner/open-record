# 12 · Comando `impact`

| Campo | Valor |
| --- | --- |
| Depende de | 01, 04, 05, 06, 09, 10, 11 |
| Habilita | 17, 18 |
| Costo | Medio |
| Valor | Alto |
| Tipo | Binario |

## Contexto

Antes de cambiar o agregar un record hacía falta saber qué podía verse afectado, y la única forma era leer
el store a mano o buscar texto — una búsqueda nunca prueba que no se le escapó nada. Esta tarea agrega
`impact`, que sigue los vínculos ya declarados y validados del store en vez de buscar texto.

## Qué se construyó

- **`openrecord impact <record>`**: devuelve el vecindario completo de un record — quién lo inicia
  (`initiated_by`, solo `flow`/`process`), qué lo califica (`qualified_by`, solo funcional), a qué sirve o
  de qué depende (`served_by`/`serves`, `depends_on`/`dependents`), qué lo verifica (`verified_by`,
  `annotated_in`), su cobertura por escenario (`scenarios`) y, en una `quality`, el estado de cada ancla
  (`verification`).
- **Secciones por tipo de record**, siempre con el mismo nombre y forma; una que no aplica al tipo se
  **omite** (nunca vacía): spec funcional, spec `quality`, decisión tienen cada una su propio subconjunto
  (ver `docs/cli.md#impact`).
- **Profundidad de un salto para todo vínculo, salvo `depends-on`**: ese se sigue en **cadena completa y
  en las dos direcciones**, en campos separados — `depends_on` (de qué se apoya, hacia arriba) y
  `dependents` (qué depende de él, hacia abajo, lo que hay que reevaluar si cambia). Dentro de
  `served_by`, cada decisión que sirve a la spec trae su propia cadena anidada.
- **Cada record listado lleva solo `path`, `status` y `description`**, nunca el cuerpo: el consumidor es
  una IA que decide qué abrir, y `status` le dice qué es `pending` (no se puede contradecir ni romper algo
  que nadie decidió).
- **Solo vínculos declarados y validados**: nunca "mismo componente" — eso es cercanía semántica, trabajo
  de `openrecord-reconcile` sobre `map --for <carpeta>`, no de este comando determinista.
- **`validate` corre primero, sobre todo el store** (no solo el vecindario): un error en cualquier lado
  refuta (exit 1, mismos `findings` que `validate`, sin vecindario); un aviso no bloquea. El record pedido
  inexistente es error de uso (exit 2).
- **`--only <secciones>`**: recorta la salida a las secciones nombradas; la clave de identidad (`record`)
  siempre queda. Un nombre inválido para ese modo es error de uso.

## Decisiones

- **Solo JSON en esta iteración**: se midió contra Mermaid, prosa relacional y una lista de triplas con
  Claude limpio en `orlab` (48 llamadas, corregidas por script). JSON y triplas acertaron el 100%; Mermaid
  y prosa fallaron al leer atributos escritos como texto corrido (confundir `approximated` con no
  verificado). La diferencia de tokens entre formatos fue de apenas ~15%. `--human`, Mermaid y `--out`
  quedaron diferidos (ver "A futuro" original; `--human` sigue sin implementarse).
- **`depends-on` en las dos direcciones, en campos separados**: se decidió al definir la tarea, sobre lo
  que el documento original solo preveía como una sola cadena hacia arriba — `Dependents` en
  `internal/check/relations.go` ya calculaba el sentido inverso y se expuso también acá.
- **Secciones que no aplican se omiten, nunca vacías**: distinción deliberada entre "este tipo no tiene
  esto" y "lo tiene mas no hay nada".
- **`validate` corre sobre todo el store, no solo la coordenada pedida**: un vecindario sobre un store con
  errores no es confiable, y el error puede estar en una decisión lejana que sirve al record pedido. Costo
  aceptado: `impact` tarda lo que tarda `validate`.

## Fuera de alcance / diferido

- **`--human`**: vista legible para una persona. Sigue pendiente, sin tarea asignada todavía.
- **Mermaid** del vecindario y **`--out <archivo.md>`**: evaluados y diferidos junto con `--human`.
- Una profundidad configurable mayor a un salto para vínculos que no son `depends-on`: evaluada y
  descartada.
- Fusionar `impact` y `impact --files` en un solo comando sin distinguir el modo: reciben entradas y
  responden preguntas distintas (resuelto como dos modos de un mismo comando en la tarea 13, no como dos
  comandos separados).

## Criterios de aceptación

- [x] `openrecord impact <record>` devuelve JSON. `--human` queda para una iteración futura.
- [x] Cada sección de la salida tiene un test que confirma que se resuelve correctamente sobre un store de
      prueba con los vínculos correspondientes.
- [x] `depends-on` se sigue en cadena completa, con un test que cubre al menos tres niveles de
      profundidad.
- [x] El resto de los vínculos se detiene exactamente en un salto, con un test que confirma que un vínculo
      de segundo salto no aparece.
- [x] `docs/cli.md` documenta `impact` siguiendo la misma convención que el resto de los comandos.

## Commits

`a0cde7d` `c540f16` `d996aca` `098698f`

## Escenario con IA

`e2e-ai/scenarios/12-impact.md` — PASS (2026-10-16, checked 2/2, judged 3/3, run
`20261016T213948Z-12-impact`; repetido el 2026-10-20 tras cambios posteriores, run
`20261020T174521Z-12-impact`. Claude Code 2.1.296 como orlab).
