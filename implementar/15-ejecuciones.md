# 15 · Ejecuciones (`run`, núcleo)

| Campo | Valor |
| --- | --- |
| Depende de | 03, 09, 11, 13, 14 |
| Habilita | 16, 18 |
| Costo | Alto |
| Valor | Alto |
| Tipo | Binario |

## Contexto

El flujo de implementación (tarea 18) puede correr sin supervisión durante horas. Si se corta hace falta
poder retomarlo sin repetir lo hecho, y si deja huecos hace falta un lugar donde esperen con toda la
evidencia. Esta tarea construye el **núcleo** de los artefactos de ejecución; lo que depende de la tarea
18 (worktree, reportes, reintentos por tarea, retención) queda para después.

## Qué se construyó

- **`.openrecord/runs/<id>/`**, ignorado por git, hasheado, escrito solo por el binario, sin worktree
  propio todavía (ver tarea 20) y sin limpieza automática:
  - `run.json`: `id`, `scope` tal como se pidió, `commit` (`HEAD` al arrancar, o `""` fuera de un
    repositorio git), `state` (`open`), `created`.
  - `plan.json`: la foto del alcance al arrancar — `records` (`path`, `status`, `description`,
    `body_hash`, `compliance`, `possibly_stale`, `verified_by`, `scenarios`) y `outside_scope` (records de
    afuera del alcance de los que depende algo adentro, por `served_by`/`depends_on`, y que **no** están
    `up_to_date` ahora mismo — informativo, nunca bloquea `run start`).
  - `tasks.json`: solo la **definición** de las tareas (`id`, `title`, `records`, `depends_on`), más un
    `history` de replanes; nunca el estado de una tarea ni sus archivos.
  - `tasks/<task-id>.json`: la **ejecución** de esa tarea — `state`, `started` (fecha + foto de archivos
    git-modificados con hash), y en `done` los archivos (`modified`/`created`/`deleted`, deducidos por git,
    nunca dados por el agente) y los checks.
  - `gaps/<id>.json`: un archivo por hueco.
- **`run start --scope <rutas...>`** (obligatorio por ahora): valida todo el store primero, resuelve el
  alcance (grupos o records puntuales, unión sin repetir; cualquier ruta inexistente falla todo el
  arranque), y escribe `run.json`/`plan.json` usando la misma función que `compliance status`
  (`complianceSnapshot`) — nunca una segunda implementación que pudiera responder distinto.
- **`run status`**: fusiona definición + ejecución de cada tarea y reporta `drift` contra el plan
  (`changed`, `added`, `removed`); **nunca actualiza `plan.json`** — decidir si seguir o arrancar de nuevo
  es de la persona.
- **`run list`**: listado barato de todas las ejecuciones, sin validar el store ni abrir plan/tareas.
- **`run tasks set`**: la IA propone la lista completa de tareas (desde `--file` o stdin); el binario
  hace cumplir cobertura (todo record que necesita trabajo tiene tarea), contención (solo records del
  plan), inmutabilidad de lo `done`, `depends_on` válido y sin ciclos, ids únicos. Ningún `state` se acepta
  en la entrada. Cada `set` exitoso guarda la versión anterior en `history`.
- **`run task start`**: mueve una tarea `pending` a `in_progress`, rechaza si una dependencia no está
  `done` o si otra tarea del run ya está `in_progress` (las tareas de un run corren **en serie**: comparten
  el checkout). Guarda una foto (`snapshot`) de los archivos que git ve cambiados en ese momento.
- **`run task done`**: **sin `--files`** — el binario clasifica `created`/`modified`/`deleted` comparando
  la foto de `start` contra git. Corre, con el mismo motor que `check run`, la unión de los checks
  `{files}` (completados con `modified ∪ created`) y los checks cuyo `enforces` cubre los records de la
  tarea (estos últimos corren aunque no haya cambios). Si todo pasa, escribe `done` y mueve el estado; si
  algo falla, no escribe nada y la tarea sigue `in_progress` para poder reintentar desde la misma foto.
  **No escribe todavía en `compliance`** — eso queda para la tarea 18.
- **Huecos**: `run gap add` (con `--id`, `--question`, `--kind` igual a los valores de `blocked_by` de
  `compliance`, `--evidence` validado como record, `record#ancla` o `file:line`, `--blocks` sobre tareas no
  terminadas), `run gaps` (todos los abiertos, de cualquier run, sin que quien llama conozca la estructura
  interna), `run gap resolve --record` (exige un record `accepted`; las tareas que bloqueaba vuelven a
  `pending`).

## Decisiones

- **Un run no es una sesión**: es una unidad de trabajo (alcance → entrega → PR), puede durar varias
  sesiones y una sesión puede lanzar varios runs.
- **Sin worktree en esta tarea**: el run trabaja sobre el checkout actual; el worktree aislado es un
  mecanismo aparte (tarea 20). Por eso la foto y el desvío de `run status` pesan más de lo que pesarían con
  worktree.
- **`compliance`/`possibly_stale` de la foto salen de la misma función que `compliance status`**: nunca
  una segunda implementación que pudiera responder distinto para el mismo record.
- **Archivos de una tarea: los deduce el binario, nunca el agente** — `run task done` no tiene `--files` a
  propósito.
- **`tasks.json` (definición) y `tasks/<id>.json` (ejecución) son archivos separados**: un replan
  (`run tasks set`) nunca contiende con lo que `run task start`/`done` escriben, y viceversa.
- **El run nunca escribe en `compliance` todavía**: solo lo lee, para la foto. Qué evento escribe cada paso
  (`run task done` → ¿`implemented`/`adopted`/`verified`? un hueco → ¿`blocked`?) se decide con la tarea
  18, cuando el flujo completo esté claro.
- **Sin traza de lo consultado todavía**: `search`, `review` e `impact` no escriben en ninguna tarea (son
  consultas sin efecto secundario). Leer nunca recibe `verified` — eso pisa lo que el documento original de
  esta tarea sugería, y queda resuelto así. La traza y la restricción de lecturas durante un run van a la
  tarea 19.
- **`--scope` es obligatorio por ahora**, pero el código separa "resolver el alcance" del resto de
  `run start` (`resolveScope`), precisamente para que una versión futura pueda calcularlo sola cuando se
  omite (ver tarea 21).
- **Sin retención todavía**: no se borra ninguna ejecución automáticamente; se decide más adelante, con
  más experiencia acumulada.

## Fuera de alcance / diferido

- **Worktree aislado por ejecución**: tarea 20.
- **Escritura en `compliance`** desde `run task done`: tarea 18.
- **Traza de lo consultado y restricción de lecturas durante un run**: tarea 19.
- **`reviews/`, `findings/`, `report.md`, límite de reintentos por tarea, retención configurable**:
  dependen del orquestador de la tarea 18, que todavía no existe.
- **Entregas por presupuesto, partición por tamaño, `expected_files`**: diseño ya cerrado (ver
  "Notas de diseño originales" en la tarea 18), implementación pendiente junto con esa tarea.

## Criterios de aceptación

- [x] `run start --scope <alcance>` crea el id de ejecución y los archivos iniciales (`run.json`,
      `plan.json`), con tests sobre el cálculo del plan y el rechazo de una ruta inexistente.
- [ ] El worktree hermano en la ubicación correcta: no implementado en esta tarea (tarea 20).
- [x] `run task done` rechaza marcar una tarea como terminada si sus checks no pasaron, con un test.
- [x] `run gaps` devuelve los huecos de todas las ejecuciones recientes sin que quien lo llama conozca la
      estructura interna de `runs/`.
- [x] `run gap resolve` marca el hueco como resuelto y las tareas que bloqueaba quedan identificables como
      retomables.
- [ ] Retención de ejecuciones viejas: diferida (ver arriba), sin tarea asignada todavía.
- [ ] Partición en entregas por presupuesto: diferida a la tarea 18 (el diseño está cerrado; la
      implementación todavía no).
- [ ] `run task reject` (incrementar `attempts`, bloquear al llegar al límite): diferido a la tarea 18,
      según lo decidido al definir esta tarea — no existe en el núcleo.

## Commits

`56e83f6` `df60190`

## Escenario con IA

`e2e-ai/scenarios/15-runs.md` — PASS (2026-10-21, checked 5/5, judged 2/2, run
`20261021T014151Z-15-runs`, Claude Code 2.1.296 como orlab).
