# 19 · Garantía de que se ejecutan los comandos de openrecord

| Campo | Valor |
| --- | --- |
| Depende de | 03, 13b, 14 |
| Habilita | 18 |
| Costo | Medio |
| Valor | Alto |
| Tipo | Binario / Integración |

> Nació al definir la 14. Todavía sin definir en detalle: lo de abajo es el contexto y lo que hay que
> averiguar.

## Contexto

Las skills dicen qué comandos correr ("antes de entregar, `openrecord check run --files …`"), pero una
skill es una instrucción: el agente puede no cargarla (pasó en la 12: su `description` no cubría el
pedido) o cargarla y saltearse un paso. Hoy eso no está garantizado.

Regla de fondo, la misma de la 13b: **lo que tiene que pasar sí o sí no puede depender de que el agente
se acuerde.** La skill explica el porqué; un portón garantiza el qué.

| Capa | Mecanismo | ¿Garantiza? | Estado |
| --- | --- | --- | --- |
| Instrucción | Skills | No (probabilístico) | Hecho |
| Portón | Algo que corre solo y corta si no se cumplió | Sí | Parcial |

Portones que ya existen: el hook `PreToolUse` de la 03 (bloquea editar `.openrecord/` a mano) y los
códigos de salida de `validate` y `check run` (dicen la verdad, pero solo si alguien los corre).

## Qué hay que averiguar (documentación oficial de Claude Code)

- Qué eventos de hook existen y cuáles pueden **bloquear** o forzar al agente a seguir (`Stop`,
  `SubagentStop`, `PostToolUse`, `UserPromptSubmit`, …), con qué contrato (exit code 2, JSON
  `decision`), y cómo evitar bucles (`stop_hook_active`).
- Si un hook `Stop` puede saber qué comandos ya corrió el agente en la sesión (transcript) o si alcanza
  con recalcular desde git qué cambió.
- Si `PostToolUse` sirve para correr los checks `per-change` sobre el archivo recién editado.
- Dónde se configuran (proyecto / usuario / plugin / política administrada), si el agente o la persona
  pueden saltearlos, y si corren dentro de subagentes y en `claude -p`.

## Lo que dice la documentación (investigado al crear la tarea)

Fuentes: [Hooks guide](https://code.claude.com/docs/en/hooks-guide.md) y
[Hooks reference](https://code.claude.com/docs/en/hooks.md). Hay que volver a confirmarlo al definir la
tarea: la lista de eventos cambia entre versiones.

- **Pueden bloquear:** `PreToolUse`, `UserPromptSubmit`, `Stop`, `SubagentStop`, `PreCompact`,
  `TaskCompleted`, `PostToolBatch`, entre otros. **No bloquean:** `SessionStart`, `Notification`,
  `PostToolUse` (corre después de la herramienta; ver abajo).
- **`Stop` / `SubagentStop`:** se bloquea con exit code 2 (stderr es el mensaje) o con JSON
  `{"decision": "block", "reason": "..."}`; el `reason` vuelve a Claude y sigue trabajando.
  `stop_hook_active: true` en la entrada indica que ya es una continuación forzada: salir con 0 para no
  entrar en bucle. Claude Code fuerza el corte tras **8** bloqueos seguidos sin una herramienta en el
  medio (`CLAUDE_CODE_STOP_HOOK_BLOCK_CAP`). Timeout por defecto de un hook de comando: 600 s.
- **Entrada del hook:** `session_id`, `cwd`, `transcript_path`, `hook_event_name`; en `Stop`,
  `stop_hook_active` y `last_assistant_message`. **No hay** una primitiva documentada para "¿ya corrió
  el comando X?"; se puede leer el `transcript_path` (JSONL) por cuenta propia, o recalcular desde git.
- **`PostToolUse`:** no bloquea la herramienta, pero con `continueOnBlock: true` el `reason` vuelve a
  Claude en el mismo turno: sirve para "corré los checks per-change del archivo recién editado y, si
  fallan, arreglalo".
- **Dónde se configuran:** usuario (`~/.claude/settings.json`), proyecto (`.claude/settings.json`),
  local, plugin, frontmatter de una skill o subagente, y políticas administradas. `disableAllHooks`
  apaga los propios, **no** los administrados; `allowManagedHooksOnly` deja solo los administrados. Los
  hooks de proyecto requieren confiar en el workspace.
- **Subagentes y `claude -p`:** `Stop` corre en la conversación principal; el fin de un subagente
  dispara `SubagentStop`. En `-p` los hooks de herramientas y `Stop`/`SubagentStop` corren igual.
- **Ojo:** los hooks de un mismo evento corren en paralelo; `${CLAUDE_PROJECT_DIR}` apunta siempre al
  checkout original, aun dentro de un worktree (el directorio vivo está en `cwd`).

Consecuencia para openrecord: un `Stop` puede garantizar `check run` al terminar; la persona puede
apagarlo con `disableAllHooks` salvo que sea una política administrada; y `check run` lento en cada
`Stop` pesa (8 bloqueos como tope, 600 s por hook).

## Candidatos

1. **Hook `Stop`**: al querer terminar, corre `openrecord check run --files <lo que cambió según git>`;
   si falla, devuelve el error y el agente sigue.
2. **Hook `PostToolUse`** sobre `Edit`/`Write`: corre los checks `per-change` del archivo editado.
3. **`run task done`** (tarea 15): portón dentro del flujo de implementación.
4. **`pre-commit` / CI** del proyecto: la última red, no depende de ningún agente.

Instalación: `openrecord skills --emit <dir> --guard` ya instala el hook de la 03; los nuevos irían por
el mismo camino.

## Desde la 15

- **Restringir lecturas durante un run:** un hook `PreToolUse` sobre la lectura de `.openrecord/**` que
  permita solo los records que salieron del plan, de `impact` o de las búsquedas de ese run (servidos con
  veredicto `governs`/`contradicts`). `map` no cuenta como fuente: `search` ya acota con `--for`.
- **Traza de lo consultado por tarea** (ids de `search`, llamadas a `impact`), que es lo que el hook
  necesita. Sin efectos secundarios en los comandos de consulta: hay que decidir cómo se registra.
- Un hook `Stop` que no deje terminar con tareas de un run abiertas.

## Abierto

- Qué se fuerza y qué no (¿también `compliance log` al cerrar? ¿`impact --files` antes de editar?).
- Cómo no volverlo insoportable (checks lentos en cada `Stop`, sandboxes).

## Notas de diseño originales (de `new/12-garantias.md`)

Propuesta original, no decidida con el mismo rigor que la investigación de hooks de arriba — conservada
porque dos de sus barreras todavía no tienen portón concreto ni en esta tarea ni en ninguna otra.

**Panorama completo de garantías** (algunas ya resueltas por tareas anteriores, otras siguen abiertas):

| Barrera | Cómo | ¿Dónde quedó |
| --- | --- | --- |
| El flujo no escribe records | Hooks y permisos del agente bloquean `record write`/`record edit` durante una ejecución | **Abierto** — depende de `run.json` (tarea 15) para saber que hay una ejecución en curso; diseñado en la tarea 03, no implementado todavía |
| El flujo no edita la configuración de un check | `checks.json` declara los archivos de `config`; un hook bloquea su edición solo durante una ejecución | **Abierto**, mismo motivo que la fila anterior |
| Nada se entrega sin verificar | Hooks que impiden dar una tarea por terminada si falla un check | Resuelto en parte por `run task done` (tarea 15), que ya se niega si los checks fallan; el hook adicional de esta tarea es el respaldo si alguien lo saltea |
| Nada entra sin revisión | Trabajo en rama propia; el resultado es un PR | Ya es el diseño de la tarea 18: "sin intervención" aplica al proceso, no a la aprobación final |
| Cambios en el store aprobados por personas | CODEOWNERS, o un check de CI sobre `.openrecord/` | **Candidato sin desarrollar**: ninguna tarea lo cubre todavía: es una guarda a nivel de PR/repositorio, no de agente, y queda abierta para cuando haga falta |
| Sin bucles | Intentos acotados por tarea | Resuelto: límite de 3 intentos, tarea 18 |
| Quien escribe no audita | Modelos distintos para implementar y para verificar | Resuelto: regla no negociable de la tarea 18 |

El único ítem de esta tabla sin ningún lugar asignado todavía es **"cambios en el store aprobados por
personas"** vía CODEOWNERS o un check de CI — queda como candidato para cuando se retome esta tarea.
