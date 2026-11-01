# 08 · Catálogo de verificación

| Campo | Valor |
| --- | --- |
| Depende de | 04, 07 |
| Habilita | 09, 16, 17 |
| Costo | Alto |
| Valor | Alto |
| Tipo | Docs |

## Contexto

`docs/concerns.md` ya viajaba con el binario como túnel de preguntas de "¿qué decidiste?". Hacía falta un
segundo documento del mismo tipo para la pregunta "¿cómo se comprueba que el código cumple lo que
decidiste o lo que la spec promete?".

## Qué se construyó

- **`docs/verification.md`**, embebido en el binario, nunca dentro de un proyecto, con **tres puertas**:
  specs funcionales (por tipo: `flow`, `process`, `lifecycle`, `rule`, más dos entradas transversales de
  cobertura y calidad de tests), specs `quality` (una entrada por atributo: latencia, throughput,
  disponibilidad, durabilidad, recuperación, resiliencia, uso de recursos, tamaño de artefacto, tiempo de
  arranque, rendimiento web, frescura de datos, accesibilidad, usabilidad, compatibilidad, exposición de
  seguridad), decisiones (una entrada por cada uno de los sesenta temas de `docs/concerns.md` revisado en
  la tarea 07, organizadas por concern — sin columna de pregunta propia, porque esa pregunta ya está en
  `concerns.md`).
- **Forma fija de cada entrada**: pregunta (solo puertas 1 y 2), a qué tipo de record se liga, qué tiene
  que decir el record para ser verificable, etapa típica, ejemplos de herramientas (nunca una elección).
- **Tres clases de verificación**, mostradas separadas en cualquier reporte posterior: check completo,
  check parcial + auditoría, y solo humana — para que nadie confunda "pasó el check" con "lo juzgó un
  agente" con "no lo verificó nadie".
- **`openrecord verification [KEY]`**: imprime el documento completo verbatim, o una entrada puntual por
  `KEY` (`<tipo-de-spec>/<aspecto>`, un atributo de `quality` a secas, o `<concern>/<tema>`). Un `KEY`
  desconocido es error de uso que sugiere las claves de la puerta más cercana.
- **Sección final** sobre lo que ningún mecanismo demuestra: ausencia de errores, que la spec sea correcta,
  ausencia de vulnerabilidades.

## Decisiones

- **Se imprime verbatim, nunca reconstruido** a partir de entradas parseadas: el documento es el
  entregable, y reconstruirlo arriesgaría a que se desalinee con lo que de verdad se envía con el binario
  — mismo criterio que `openrecord concerns`.
- **La puerta 3 no repite la pregunta de cada tema**: ya está en `docs/concerns.md`; repetirla la
  duplicaría en sesenta filas.
- **Las herramientas listadas son siempre ejemplos**, nunca una elección automática: la elección es de una
  persona.

## Fuera de alcance

- Que el catálogo liste herramientas para elegir automáticamente.
- Un mecanismo que detecte automáticamente qué entradas del catálogo "ya están cubiertas" en un proyecto:
  eso es, precisamente, lo que hacen `impact` y `impact --files` (tareas 12 y 13) sobre el store real, no
  el catálogo en abstracto.

## Criterios de aceptación

- [x] El catálogo existe como documento embebido en el binario, con las tres puertas y todas sus entradas,
      siguiendo la forma de campos acordada.
- [x] `openrecord verification [KEY]` lo imprime verbatim, completo o por entrada.
- [x] Cada entrada de la puerta 3 corresponde exactamente a un tema de `docs/concerns.md` revisado (tarea
      07); no hay temas sin entrada ni entradas sin tema.
- [x] El documento incluye la sección "lo que ningún mecanismo demuestra".
- [x] La documentación aclara que el catálogo nunca entra a un proyecto y que las herramientas listadas
      son solo ejemplos.

## Commits

`90d9853` `09582eb` `56d3ce5`

## Escenario con IA

`e2e-ai/scenarios/08-verification.md` — PASS (2026-10-14, checked 6/6, judged 4/4, run
`20261014T122531Z-08-verification`, Claude Code 2.1.296 como orlab).
