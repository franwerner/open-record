# 05 · Relaciones entre decisiones

| Campo | Valor |
| --- | --- |
| Depende de | — |
| Habilita | 07, 09, 12, 13 |
| Costo | Bajo |
| Valor | Alto |
| Tipo | Formato |

## Contexto

El formato dejaba las relaciones entre records deliberadamente afuera: una lista de referencias a mano se
pudre en silencio cuando algo se renombra. Ese motivo deja de aplicar en cuanto existe un binario que
valida cada referencia al escribir y en cada `validate`. Esta tarea reabre esa decisión para dos campos,
los dos opcionales y solo en una decisión.

## Qué se construyó

- **`serves`** (decisión → spec): lista de rutas de specs, funcionales o `quality`, a las que la decisión
  sirve. Opcional — una decisión sin spec detrás (conocimiento del equipo, costo, licencias) queda sin
  `serves`, con el porqué desarrollado en `## Context`.
- **`depends-on`** (decisión → decisión): lista de otras decisiones que tienen que seguir `accepted` para
  que esta tenga sentido. Significado único y estricto: "si la otra cambiara, ¿esta habría que
  reevaluarla?". Nunca un "relacionado" genérico ni el vínculo herramienta→decisión (eso es
  `justified-by` en `checks.json`, tarea 09).
- **Las dos apuntan a la ruta del record entero**, nunca con `#ancla` (no hay nada más angosto que una
  decisión para apuntar), sin importar en qué componente o concern vive el destino.
- **Inversas calculadas, nunca escritas**: `record write`/`record edit` devuelven `dependents` (toda
  decisión que depende de esta, directa o transitivamente) sobre una decisión, y `served_by` (toda
  decisión cuyo `serves` nombra esta spec) sobre una spec — informativo, nunca bloquea.
- **Validaciones**: `serves-not-found`, `serves-not-spec`, `depends-on-not-found`,
  `depends-on-not-decision`, `depends-on-pending` (apoyarse en algo no `accepted` no asienta nada),
  `depends-on-cycle` (directo o transitivo).

## Decisiones

- **Por qué es seguro reabrir esto y no una relación genérica**: el binario valida cada referencia en el
  momento de escribir y en cada `validate`, así que una referencia rota se reporta, nunca queda en
  silencio — exactamente el motivo que originalmente las mantuvo afuera del formato.
- **Solo estas dos relaciones, nada de "relacionado"**: un vínculo genérico se usa de más y el grafo deja
  de significar algo; dos decisiones del mismo componente y concern ya aparecen juntas por la estructura
  del store sin necesidad de un campo.
- **Decisión sin `serves` no se marca mecánicamente**: lo juzgan las personas y las skills de definición,
  no un chequeo — una regla de "mínimo N palabras" en `## Context` se cumpliría con relleno.

## Fuera de alcance

- Una relación genérica de "relacionado" o cualquier vínculo entre decisiones distinto de `depends-on`.
- Enlazar una decisión hacia atrás desde la spec: la relación vive solo en la decisión.

## Criterios de aceptación

- [x] `record write` y `record edit` aceptan `serves` y `depends-on` en una decisión, ambos opcionales.
- [x] `validate` reporta una referencia rota en cualquiera de los dos campos, con un test por campo.
- [x] `validate` reporta `depends-on` hacia una decisión `pending`, con un test.
- [x] `validate` detecta un ciclo en `depends-on`, directo y transitivo.
- [x] `docs/decision-record.md` documenta los dos campos y retira la mención de "relations" como algo
      deliberadamente ausente del formato.
- [x] Existe un decision record que documenta la reapertura de las relaciones entre decisiones.

## Commits

`3b0dbfb` `1ba2881`

## Escenario con IA

`e2e-ai/scenarios/05-relations.md` — PASS (2026-10-14, checked 6/6, judged 4/4, run
`20261014T011048Z-05-relations`, Claude Code 2.1.296 como orlab).
