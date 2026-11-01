# 13b · `check run` / `sandbox run` (era `verify`)

| Campo | Valor |
| --- | --- |
| Depende de | 09, 11, 13 |
| Habilita | 15, 18 |
| Costo | Alto |
| Valor | Alto |
| Tipo | Binario |

## Contexto

`impact`/`impact --files` devuelven la lista de checks que aplican, pero completar `{files}`, elegir cuál
correr y levantar un sandbox es mecánico, no juicio — y lo mecánico lo hace cumplir el binario (como
`validate`), no lo informa. Esta tarea nació al definir la 13 y agrega ese "correr de verdad".

## Qué se construyó

- **No hay comando `verify`**: se agregó `run` a los grupos que ya administran cada registro —
  `check run <id> | --files a b [--per-change]` y `sandbox run <id> | --files a b`. Esto da vuelta lo
  decidido en las tareas 09/11 ("el binario nunca corre checks ni sandboxes"): ahora sí corre.
- **Misma cadena que `impact --files`** para llegar de archivos a checks/sandboxes, declarada de punta a
  punta: archivo → componentes (todos los que lo contienen) → records → checks cuyo `enforces` los cubre /
  sandboxes que `observes` una de esas decisiones, `approximates` una de esas calidades, o son pedidos
  (`needs`) por uno de esos checks. Nunca se eligen sandboxes por su propio campo `components` — eso
  levantaría algo sin relación real con lo que se toca.
- **`check run`**: completa `{files}`, corre, y por cada check devuelve `result` (`passed`/`failed`/
  `skipped` con `reason`), el comando efectivo y los segundos. **El código de salida es el veredicto** (0
  solo si todo pasó). Un check con `needs` levanta su sandbox con el mismo ciclo de `sandbox run` (sin
  `exercise`), una sola vez para todos los checks que lo necesitan, y lo apaga siempre, incluso si alguno
  falló.
- **`--per-change`** corre solo los checks con `{files}` en el comando (el ciclo de edición); sin el flag,
  corren **todos**, con y sin `{files}` (el barrido "antes de entregar"). Olvidarse el flag siempre da el
  resultado más seguro, nunca uno que se saltee algo importante.
- **`check run <id>` exige `--files` si el comando tiene `{files}`**: `{files}` nunca se completa con nada
  — correría sobre todo el proyecto y un "pasó" afirmaría algo que nunca se corrió.
- **`sandbox run`**: ciclo `setup → start → ready → exercise → stop` (el paso `logs` queda fuera de esta
  tarea — ver abajo). `ready` espera `ready.http` hasta `ready.timeout` segundos (`ready.log` no está
  implementado todavía: un sandbox que lo declara falla ese paso siempre, de forma deliberada y
  documentada en vez de un intento a medias). Una vez que un paso falla, los siguientes antes de `stop` se
  reportan `skipped`; `stop` **siempre** corre. Devuelve, por sandbox, `because` (qué `observes`,
  `approximates` con su `measure`, qué checks lo `needs`) y los `steps` con resultado. El código de salida
  dice si el ciclo completó, nunca si la salida "está bien" — eso sigue siendo juicio.
- **Validado primero, sobre todo el store**, exactamente igual que `impact`.
- **Concurrencia**: los checks corren de a uno, en orden; un sandbox pedido por varios checks se levanta
  una sola vez para todos y recién ahí se apaga.

## Decisiones

- **Logs y salida guardada quedan fuera de esta tarea**: no se guarda nada en disco ni se devuelve un id;
  un check que falla reporta solo `"result": "failed"`, sin salida. El paso `logs` del ciclo de sandbox
  **no se ejecuta** (no hay dónde dejar lo capturado todavía). Consecuencia aceptada: hasta que eso
  llegue, `sandbox run` sirve para los checks con `needs`, no para revisar logs contra `observes`.
- **Timeouts de checks y de los pasos del sandbox** (`setup`, `start`, `exercise`, `stop`) no existen
  todavía — solo `ready` tiene timeout. Propuesta conversada para la siguiente iteración: 10 minutos sin
  salida (heartbeat) más un tope absoluto, resultado `timeout`, con `stop` corriendo igual.
- **`ready.log` no implementado**: se eligió fallar siempre el paso de forma explícita antes que una
  implementación a medias.

## Fuera de alcance / diferido

- **Guardar logs y salidas por invocación** (`.openrecord/logs/<id>/`, ignorado por git, últimas N,
  mismas reglas que `runs/` de la tarea 15) y un comando que los lea por id: próxima iteración, sin tarea
  numerada todavía.
- **Timeouts de checks y de los pasos de un sandbox** (más allá de `ready`): próxima iteración.
- **`ready.log`**: declarado y validado por `validate`, pero `sandbox run` todavía no sabe esperarlo.
- **Relación con `run task done`**: se resolvió en la tarea 15, que reutiliza el mismo motor de ejecución.

## Criterios de aceptación

- [x] `check run --files <archivos>` y `check run <id>` existen, resuelven checks por la cadena
      declarada, completan `{files}` y el código de salida es el veredicto.
- [x] `--per-change` corre solo los checks con `{files}`; sin el flag, corren todos.
- [x] `sandbox run <id>` y `sandbox run --files <archivos>` corren el ciclo `setup → start → ready →
      exercise → stop`, con `stop` corriendo siempre aunque un paso anterior falle.
- [x] Un sandbox pedido por varios checks se levanta una sola vez para todos.
- [ ] Logs y salidas guardados por id: no implementado en esta tarea (diferido, ver arriba).
- [ ] Timeouts más allá de `ready`: no implementado en esta tarea (diferido, ver arriba).

## Commits

`d330a78` `0eb7e42`

## Escenario con IA

`e2e-ai/scenarios/13b-run.md` — PASS (2026-10-20, checked 3/3, judged 2/2, run
`20261020T192504Z-13b-run`, Claude Code 2.1.296 como orlab).
