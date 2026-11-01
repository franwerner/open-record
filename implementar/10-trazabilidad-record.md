# 10 · Trazabilidad con anotaciones `@record:`

| Campo | Valor |
| --- | --- |
| Depende de | 01, 09 |
| Habilita | 12, 13, 18 |
| Costo | Medio |
| Valor | Alto |
| Tipo | Formato / Binario |

## Contexto

No existía forma determinística de saber qué test prueba qué escenario, ni qué regla de linter hace
cumplir qué decisión. Esta tarea define una única convención de anotación que el binario busca como texto
en el repositorio, sin ejecutar nada.

## Qué se construyó

- **Sintaxis `@record:<ruta>[#<ancla>]`**, dentro de cualquier sintaxis de comentario de cualquier
  lenguaje (el binario busca el patrón en el texto, no parsea cada lenguaje). `<ancla>` es opcional y usa
  exactamente la regla de la tarea 01; omitida, la anotación cubre el record entero (el único caso
  posible en una decisión, que no tiene anclas).
- **Dos relaciones distintas**: record → código que **gobierna** (el componente, vía `components.json`,
  ya existente) y record → código que **verifica** (las anotaciones). Una anotación nunca va en código de
  producción, solo en lo que ejecuta la verificación (test, regla, script).
- **Alcance del escaneo**: todo lo que `git ls-files --cached --others --exclude-standard` reporta (o, sin
  git, un recorrido de filesystem que salta `.git/`, `node_modules/`, `vendor/`), menos `.openrecord/` y
  cualquier `*.md`.
- **Cálculo de cobertura**: por cada spec, se arma la lista de anclas (tarea 01) y se cruza contra las
  anotaciones encontradas. Un escenario sin ninguna anotación es trabajo pendiente, no un error; una
  anotación hacia una ruta o un ancla que no existe sí lo es.
- **Validaciones**: `annotation-record-not-found`, `annotation-anchor-not-found`, reportadas en el
  `file:line` de la anotación (nunca en el record que nombra). `validate` sin `--for` reporta cada
  anotación rota del repositorio; `validate --for <coordenada>` solo las que caen dentro.
- **Consumida por `impact`** (tarea 12) como `annotated_in` (anotaciones al record entero) y `scenarios`
  (`{ancla: [file:line...]}`, `[]` si no tiene test todavía).

## Decisiones

- **Un comentario de texto, no una convención de nombres ni specs ejecutables**: un comentario es
  explícito y vive en el test; una convención de nombres es igual de frágil (renombrar rompe el vínculo
  sin avisar); specs ejecutables obligarían a que la spec sea parseable como test.
- **Nunca "tests huérfanos"**: detectar qué es un test depende del lenguaje; eso queda para la revisión
  semántica (la auditoría del flujo de implementación, tarea 18), no para esta pieza determinística.
- **`*.md` excluido del escaneo**: la propia documentación usa `@record:` en sus ejemplos, y no son
  anotaciones reales.

## Fuera de alcance

- Detectar un test sin anotación ("test huérfano") de forma mecánica.
- Verificar que una anotación de verdad prueba lo que dice probar: eso es juicio de quien revisa, no de
  esta pieza.
- Una sintaxis de anotación distinta por lenguaje: descartada a favor de un único patrón de texto.

## Criterios de aceptación

- [x] El binario escanea archivos de texto del repositorio en busca de `@record:<ruta>#<ancla>`, con tests
      que cubren al menos tres sintaxis de comentario distintas.
- [x] `validate` reporta una anotación con ruta inexistente y una con ancla inexistente, cada una con su
      propio test.
- [x] El cruce entre anclas declaradas por una spec y anotaciones encontradas queda expuesto en una forma
      que la tarea 12 consume directamente (cobertura por escenario).
- [x] La documentación explica la diferencia entre "record → código que gobierna" y "record → código que
      verifica", con ejemplos de cada relación.
- [x] Existe un decision record que documenta la convención y por qué se eligió un comentario de texto.

## Commits

`128801c` `dd7c2a9` `7079d76`

## Escenario con IA

`e2e-ai/scenarios/10-annotations.md` — PASS (2026-10-14, checked 5/5, judged 4/4, run
`20261014T174232Z-10-annotations`; repetido el mismo día tras rebuild con `check.sh` para que la skill de
captura llevara la regla de anotación, run `20261014T175154Z-10-annotations`. Claude Code 2.1.296 como
orlab).
