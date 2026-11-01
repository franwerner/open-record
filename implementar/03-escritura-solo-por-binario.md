# 03 · Escritura solo por el binario

| Campo | Valor |
| --- | --- |
| Depende de | 02 |
| Habilita | 06, 09, 11, 14, 15, 16, 17, 18 |
| Costo | Medio |
| Valor | Alto |
| Tipo | Protección |

## Contexto

Todo lo que vive dentro de `.openrecord/` solo es confiable si pasa por el binario, que valida antes de
escribir. Antes de esta tarea esa regla era una convención que un agente podía saltear con sus propias
herramientas de edición; la única defensa real era la detección después del hecho (`body-hash`/`hash`,
tarea 02). Esta tarea agrega una barrera mecánica del lado del agente, además de mantener la detección
como respaldo contra una persona editando a mano.

## Qué se construyó

- **`openrecord hook check`**: subcomando que recibe en stdin un tool call de Claude Code (`PreToolUse`) y
  devuelve la decisión en el formato nativo de Claude Code (`permissionDecision: allow|deny` dentro de
  `hookSpecificOutput`). No es la barrera — los permisos y el sandbox lo son — da el mensaje con el
  comando correcto.
  - `Edit`/`Write`/`NotebookEdit` sobre cada JSON de registro, sobre un record bajo `decisions/`/`specs/`,
    sobre un `INDEX.md`, o sobre cualquier otra cosa dentro de `.openrecord/` o `.claude/`: deniega y
    nombra el comando equivalente.
  - `Bash` cuyo texto completo (no solo la primera palabra) menciona un patrón de escritura directa
    (`tee`, `sed -i`, `cp`/`mv`/`rm`, `git checkout`/`git restore`, una redirección) contra
    `.openrecord/`/`.claude/`: deniega con una explicación — mensaje best-effort, no la barrera real.
  - Falla abierto ante cualquier cosa que no reconoce (`Read`/`Glob`/`Grep`, una herramienta nueva); falla
    **fuerte** (exit no-cero) solo si el stdin no parsea como JSON.
- **`openrecord skills --emit <dir> --guard`**: además de las skills, escribe en `.claude/settings.json`
  las tres capas de protección, mezclando sin pisar lo que ya hubiera en el archivo:
  1. `permissions.deny`: `Edit(/.openrecord/**)` y `Edit(/.claude/**)`, con el `/` inicial que ancla al
     root del proyecto (no al directorio de trabajo de la sesión).
  2. `sandbox.*`: `enabled: true`, `failIfUnavailable: true`, `allowUnsandboxedCommands: false`,
     `filesystem.denyWrite: [".openrecord/**", ".claude/**"]`, `excludedCommands: ["openrecord *"]` (el
     patrón exacto del binario, nunca uno más amplio) y `permissions.allow: ["Bash(openrecord *)"]` para
     que el comando excluido no pida confirmación cada vez.
  3. `hooks.PreToolUse`: llama a `openrecord hook check`.
  - Reporta qué agregó (`created`), qué ya estaba (`unchanged`) o qué corrigió contra un valor en
    conflicto (`updated`); correr el emit dos veces da exactamente el mismo resultado.
  - `git` queda deliberadamente **fuera** de la exclusión del sandbox: restaurar o cambiar de rama sobre
    `.openrecord/` es una acción humana, no del agente.

## Decisiones

- **Dos actores, dos mecanismos, ninguno sustituye al otro**: a un agente se le puede bloquear de raíz
  (permisos + sandbox de proceso, a nivel del sistema operativo); a una persona con acceso directo al
  filesystem solo se la puede detectar después (`body-hash`/`hash`, revisados por `validate` en
  pre-commit y en CI).
- **Solo Claude Code, por ahora**: es el único agente donde se verificaron las tres capas (permisos,
  sandbox de proceso con denegación de escritura por ruta, hooks consultables antes de cada herramienta)
  contra su propia documentación. Dos detalles de configuración exactos tuvieron que corregirse contra la
  documentación vigente al implementar (`sandbox.filesystem.denyWrite` etc. ya estaban bien; la forma del
  path en `permissions.deny` necesitaba el `/` inicial para anclar al root del proyecto).
- **El chequeo de `Bash` es best-effort, no la barrera**: no hay forma de ver "sandboxed por construcción"
  sobre un comando de shell arbitrario; el sandbox de proceso es lo que de verdad bloquea, el patrón solo
  da una explicación en vez de silencio.
- **Leer sigue permitido siempre**: solo se bloquea la escritura.

## Fuera de alcance / diferido

- **Soporte para otro agente** distinto de Claude Code: no bloquea nada que dependa de esta tarea.
- **Bloquear la edición de la configuración de un check durante una ejecución** y **bloquear
  `record write`/`record edit` durante una ejecución**: dependen de que exista `checks.json` (tarea 09) y
  de `run.json` (tarea 15) para saber si hay una ejecución en curso; quedaron fuera de esta tarea porque
  ninguno de los dos existía todavía. Siguen pendientes (ver tarea 19).
- **Detección de patrones de shell como mecanismo principal**: se descartó a favor del sandbox de proceso,
  porque un patrón siempre tiene una forma de evitarlo que no se anticipó.

## Criterios de aceptación

- [x] `openrecord hook check` existe, recibe la herramienta y la ruta o comando propuesto, y devuelve
      `allow` o `deny` con el comando correcto.
- [x] `openrecord skills --emit --guard` escribe la configuración de permisos, sandbox y el hook
      `PreToolUse` para Claude Code.
- [x] Queda verificado (escenario con IA) que un intento de `Edit` directo sobre
      `.openrecord/components.json` se bloquea en un entorno de Claude Code con esta configuración
      instalada.
- [x] La documentación explica por qué hacen falta las dos capas (bloqueo para el agente, detección para
      la persona).
- [x] Los bloqueos condicionales a una ejecución en curso quedan documentados como dependientes de las
      tareas 09 y 15, sin exigirse acá — hoy diferidos a la tarea 19.

## Commits

`c44fd99` `d54d987`

## Escenario con IA

`e2e-ai/scenarios/03-guard.md` — PASS (2026-10-13, checked 4/4, judged 4/4, run
`20261013T214007Z-03-guard`, Claude Code 2.1.296 como orlab).
