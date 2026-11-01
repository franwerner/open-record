# 06 · Actores

| Campo | Valor |
| --- | --- |
| Depende de | 02, 03 |
| Habilita | 12, 16, 17 |
| Costo | Medio |
| Valor | Alto |
| Tipo | Formato / Binario |

## Contexto

Ninguna spec declaraba **quién** la inicia: "usuario" a secas no distingue a un visitante de un
administrador, y esa diferencia es justo lo que una spec tiene que fijar. El sistema nunca es un actor —
es quien responde.

## Qué se construyó

- **`actors.json`**, misma forma que `components.json` (`version` + lista con `id`), con `kind` fijo
  (`human`, `external-system`, `time`), `description` obligatoria y `aliases` opcional.
- **`actor add` / `actor edit` / `actor remove`**: `add` exige `--kind` y `--description`; `edit`
  reemplaza `--description` y/o la lista entera de `--alias` (nunca agrega), y puede cambiar `--kind`
  salvo que rompa un `initiated-by` ya declarado (un actor `human` pasando a `external-system`
  invalidaría en silencio cada `flow` que depende de que lo inicie una persona); `remove` se niega
  mientras el actor siga en algún `initiated-by` o `@actor:` y lista dónde.
- **`initiated-by`**: obligatorio en `flow` (solo actores `human`) y en `process` (solo
  `external-system`/`time`); prohibido en `lifecycle`, `rule`, `quality` y en una decisión. Cada actor
  listado tiene que aparecer además como `@actor:<id>` en el cuerpo (`initiated-by-not-mentioned`).
- **Menciones `@actor:<id>`**: válidas en el cuerpo de cualquier spec o decisión, iniciadoras o no. Patrón
  que nunca matchea un email (el `@` tiene que estar al inicio de línea o después de espacio/paréntesis/
  comilla).
- **Advertencia de mención sin marcar**: el id, el nombre (con guiones también como espacios) o un alias
  registrado, escrito en el texto sin `@actor:`, dispara `unmarked-actor-mention`.
- **Sin jerarquía de actores**: un actor nunca hereda de otro; si un administrador también puede subir
  archivos, la spec lo dice explícitamente con los dos ids en `initiated-by`.

## Decisiones

- **El vínculo siempre sale de la spec hacia el actor**, nunca al revés: el actor no lista sus specs, eso
  lo calcula el binario (mismo principio que `serves`/`depends-on`, tarea 05).
- **`actor edit` puede cambiar `kind`, pero con el mismo resguardo que `remove`**: se descubrió al
  construir que permitir el cambio sin control rompería en silencio cada `flow`/`process` que ya asumía la
  clase anterior; se agregó la validación cruzada contra `initiated-by` en vez de prohibir el cambio.
- **Lo único fuera del control determinístico** es una palabra que no es actor ni alias declarado ("el
  comprador" sin ese alias): queda para revisión semántica (`reconcile`/`audit`), que se resuelve
  agregando el alias — el control determinístico se amplía con el uso, no de antemano.

## Fuera de alcance

- Jerarquía o herencia entre actores: evaluada y descartada.
- Permisos como mecanismo del binario (roles, scopes, control de acceso real): los actores son
  vocabulario para specs y decisiones, no un sistema de autorización.
- Resolver automáticamente una mención en prosa que no usa `@actor:`: queda para la revisión semántica.

## Criterios de aceptación

- [x] `actor add` y `actor remove` existen, con el bloqueo de `remove` cuando el actor sigue mencionado,
      listando dónde.
- [x] `record write` y `record edit` validan `initiated-by` en `flow` y `process`, lo rechazan en los otros
      tres tipos y en una decisión, y validan la clase del actor contra el tipo de spec.
- [x] El parser de menciones reconoce `@actor:<id>` en cualquier sección de una spec y de una decisión, sin
      falsos positivos sobre emails.
- [x] `validate` emite la advertencia de mención sin marcar usando `aliases`, con un test que la dispara y
      uno que confirma que un alias registrado la evita.
- [x] La documentación del formato describe `actors.json`, `initiated-by` y `@actor:` con la tabla de
      validaciones completa.

## Commits

`feefe34` `d605f66`

## Escenario con IA

`e2e-ai/scenarios/06-actors.md` — PASS (2026-10-14, checked 7/7, judged 4/4, run
`20261014T025121Z-06-actors`, Claude Code 2.1.296 como orlab).
