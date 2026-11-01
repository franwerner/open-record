# 01 · Anclas por nombre

| Campo | Valor |
| --- | --- |
| Depende de | — |
| Habilita | 04, 06, 09, 10, 12, 13 |
| Costo | Bajo |
| Valor | Alto |
| Tipo | Formato / Binario |

## Contexto

Antes de esta tarea, lo único que anclaba algo dentro de un record era el número de paso de una rama de
`flow` (`- **[3] condición**`), validado contra `## Main flow`. No existía manera de apuntar, desde
afuera de un record, a una parte interna de él (un escenario puntual, una transición de `lifecycle`), y
varias piezas posteriores (calidad, trazabilidad, `impact`) necesitaban exactamente eso.

## Qué se construyó

- **Slug determinístico**, igual para todo lo que se ancla: minúsculas, acentos y diacríticos quitados
  por descomposición Unicode (no por una tabla escrita a mano, para que el resultado nunca cambie), todo
  lo que no es letra o número pasa a `-`, guiones repetidos colapsados y recortados en los extremos. Una
  letra sin descomposición (`ß`, `ø`, `ł`) se deja tal cual.
- **Qué es anclable, uniforme para todo tipo de spec**: cada `### Scenario: <nombre>` es anclable en
  `flow`, `process`, `rule` y también `quality` — las cuatro comparten el mismo `## Scenarios` del núcleo
  y el mismo espacio de anclas, sin un identificador especial para calidad (`[Qn]` se evaluó y se
  descartó: una spec `quality` ancla igual que cualquier otra, por el nombre del escenario). Un
  `lifecycle` ancla además cada transición de `## States and transitions` como `<origen>-to-<destino>`,
  con el mismo slug en cada lado. Una decisión no reparte su contenido en partes direccionables: se
  referencia por el archivo entero, sin `#`.
- **Un solo espacio de nombres por record**: un escenario y una transición compiten por el mismo espacio,
  así que un escenario que slugea a `pending-to-paid` colisiona igual con una transición entre esos
  estados que con otro escenario.
- **Validación en escritura y en `validate`**: `record write` y `record edit` rechazan dos anclas
  iguales dentro del mismo record (`duplicate-anchor`), y un nombre sin ninguna letra o número que no deja
  ancla (`empty-anchor`); `validate` repite el mismo control sobre un record ya existente.
- **`anchors` en la salida de `record write`**: una escritura exitosa sobre una spec devuelve la lista de
  anclas (escenarios y, en un `lifecycle`, transiciones) en el orden del documento — la forma en que
  cualquier referencia futura las usa. Una decisión no lleva este campo (no tiene anclas que dar).

## Decisiones

- **Por qué nombre y no número de paso**: insertar un paso renumera los siguientes y mueve en silencio
  cualquier referencia que usara el número viejo; un nombre no se mueve al insertar contenido alrededor, y
  si se renombra o se borra, la referencia queda rota de forma detectable. Los números de paso siguen
  existiendo solo para anclar ramas dentro de la misma spec (`## Branches`), que es un mecanismo distinto
  y anterior a esta tarea.
- **Acentos por descomposición Unicode, no por tabla**: una tabla escrita a mano podría crecer y mover
  anclas ya existentes; la descomposición Unicode es estable por definición.
- **`quality` ancla igual que el resto** (sin `[Qn]`): se consideró un identificador propio para calidad,
  pero habría introducido una segunda regla de anclaje en el store. Un solo namespace de anclas, sin
  casos especiales por tipo, es más simple de validar y de leer.
- **El ancla nunca se guarda**: todo consumidor la recalcula a partir del nombre, así que la regla del
  slug queda fija para siempre — cambiarla rompería cada referencia ya escrita en el repositorio.

## Fuera de alcance

- Un comando dedicado a listar las anclas de un record: se expone como parte de la salida de los comandos
  que lo necesitan (`record write`, luego `impact`, trazabilidad), nunca como comando propio.

## Criterios de aceptación

- [x] La función de slug está implementada y cubierta por tests con acentos, mayúsculas, signos de
      puntuación y guiones repetidos.
- [x] `record write` y `record edit` rechazan un record con dos escenarios o dos transiciones que
      produzcan el mismo ancla (`duplicate-anchor`), y un nombre sin ancla posible (`empty-anchor`).
- [x] `validate` reporta la misma colisión sobre un record ya existente.
- [x] La documentación del formato (`docs/capability-spec.md`) explica la regla de anclaje, incluida su
      aplicación uniforme a `quality` y a los escenarios de `rule`.
- [x] Tests cubren la generación de anclas para escenarios y para transiciones.

## Commits

`a08fecc` `bb6b559`

## Escenario con IA

`e2e-ai/scenarios/01-anchors.md` — PASS (2026-10-13, checked 7/7, judged 3/3, run
`20261013T185536Z-01-anchors`, Claude Code 2.1.296 como orlab). Un escenario anterior, manual, también
quedó registrado como PASS el 2026-10-10.
