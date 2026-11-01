# 07 · Revisión de `docs/concerns.md`

| Campo | Valor |
| --- | --- |
| Depende de | 04 |
| Habilita | 08, 16, 17 |
| Costo | Medio |
| Valor | Medio |
| Tipo | Docs |

## Contexto

Con la spec `quality` (tarea 04) y los actores (tarea 06), varios temas del catálogo de concerns
preguntaban cosas que ya tenían un lugar mejor: un umbral medible es ahora una spec `quality`, y quién
puede hacer qué es comportamiento con sus propios actores. Esta tarea revisó los once concerns y sus
temas, concern por concern.

## Qué se construyó

- **`docs/concerns.md` reescrito en inglés**, con **diez concerns** (se disuelve `quality`) y **sesenta
  temas** (49 originales − `nfr-performance` + doce nuevos: `structure/code-health`,
  `structure/language-and-runtime`, `structure/dependency-policy`, `structure/shared-code`,
  `structure/code-generation`, `domain-logic/i18n-l10n` movido y ampliado, `runtime/state-management`,
  `runtime/scaling` movido, `data/data-stores`, `data/tenancy`, `security/data-protection`,
  `delivery/build`, `delivery/release`, `delivery/environments` — doce nuevos netos sobre la cuenta de
  "+1 tema" porque dos de ellos son movimientos, no altas).
- **Disolución del concern `quality`**: el nombre chocaba con el tipo de spec homónimo. `nfr-performance`
  se retira (el objetivo pasa a la spec `quality`, el mecanismo ya tenía tema propio);
  `scalability` se mueve a `runtime/scaling`; `i18n-l10n` se mueve a `domain-logic/i18n-l10n`.
- **18 temas reformulados** para preguntar solo por el mecanismo (lo que nota el actor quedó en spec
  funcional, lo que se mide con umbral quedó en spec `quality`), más una pasada de redacción general para
  que cada pregunta valga en cualquier capa (backend, frontend, CLI, librería) y no solo en un servidor.
- **`internal/catalogue/catalogue_test.go`** actualizado a 10 concerns.
- **Sin concern `stack` ni tema `verification-tooling`**: evaluados y descartados — agrupar por "nombra
  una tecnología" o "es una herramienta de verificación" se traga medio store y esconde el porqué.

## Decisiones

- **La regla de separación**: ¿qué recibe o ve un actor? → spec funcional. ¿Qué se mide, con umbral? →
  spec `quality`. ¿Con qué mecanismo y por qué ese? → decisión, archivada bajo un concern.
  `domain-logic/invariants` ya estaba escrito así y fue el modelo a seguir.
- **Filtro de cuatro preguntas para cada tema nuevo**: general (vale para cualquier capa), es una decisión
  (hay alternativas reales), es frecuente, no está cubierta por un tema existente. Un candidato evaluado y
  descartado por no pasar el filtro: `domain-logic/time-and-money` (se repartió entre `i18n-l10n` y
  `data-modeling`).
- **Un proyecto con un concern `quality/` propio no se rompe**: un concern de proyecto nunca depende de
  que el catálogo lo tenga; solo pierde la descripción por defecto para ese nombre.

## Fuera de alcance

- Tocar `e2e/navigation_test.go` (usa `data-lifecycle`, que no cambió de nombre ni de alcance).
- Migrar automáticamente los records de un proyecto que ya usa un nombre retirado o movido
  (`quality/scalability`, `quality/i18n-l10n`): queda para quien mantiene ese proyecto.

## Criterios de aceptación

- [x] `docs/concerns.md` queda reescrito en inglés con los diez concerns, los sesenta temas, las líneas en
      itálica que corresponden, y sin el concern `quality`.
- [x] `internal/catalogue/catalogue_test.go` verifica 10 concerns en vez de 11.
- [x] Existe un decision record que documenta por qué se disuelve `quality` y adónde se mueve cada uno de
      sus tres temas.
- [x] Queda constancia, en el propio documento, de que un proyecto con un concern `quality/` propio no se
      rompe por esta revisión.
- [x] La tabla de veredictos coincide con el contenido final de `docs/concerns.md`.

## Commits

`c2b9d84` `2c841aa`

## Escenario con IA

`e2e-ai/scenarios/07-concerns.md` — PASS (2026-10-14, checked 8/8, judged 4/4, run
`20261014T104349Z-07-concerns`, Claude Code 2.1.296 como orlab).
