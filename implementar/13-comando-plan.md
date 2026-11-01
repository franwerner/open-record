# 13 · `impact --files` (era `plan`)

| Campo | Valor |
| --- | --- |
| Depende de | 09, 10, 11 |
| Habilita | 15, 18 |
| Costo | Alto |
| Valor | Alto |
| Tipo | Binario |

## Contexto

`impact` (tarea 12) responde "¿qué está conectado con este record?", a partir de un record. Hacía falta
la pregunta inversa: dado que se van a tocar estos archivos de código, ¿qué hay que cumplir y cómo se
comprueba? Esta tarea responde esa pregunta — **no** como un comando nuevo, sino como un segundo modo de
`impact`.

## Qué se construyó

- **No hay comando `plan`**: es `openrecord impact --files <archivo...>`, un segundo modo sobre el mismo
  motor que el modo record de la tarea 12, con la misma validación previa de todo el store y el mismo
  `--only` para recortar secciones.
- **`files`**: mapea cada ruta pedida a **todos** los componentes cuya ruta declarada la contiene (nunca
  solo el más específico) — sin caso especial para `root` (`.`): si se declaró como `.`, cubre todo por el
  mismo mecanismo que cualquier componente más angosto, y ambos aparecen a la vez.
- **`decisions`**: agrupa por componente toda decisión filed bajo uno que posee al menos un archivo
  pedido, con `path`, `status`, `description`, `verified_by`, `annotated_in` — nunca `depends_on`/
  `dependents`/`serves` (son relaciones entre decisiones, no lo que un bloque de archivos tiene que
  respetar).
- **`specs`**: toda spec que nombra uno de esos componentes, más — un salto más — toda `quality` cuyo
  `qualifies` nombra una spec ya incluida así, aunque sus propios `components` no toquen ninguno de los
  pedidos (una `quality` rara vez vive en el mismo componente de lo que califica). Cada spec aparece una
  sola vez, con `verified_by`, `annotated_in`, `scenarios` (si tiene anclas) y, si es `quality`,
  `qualifies` + `verification`.
- **Vocabulario unificado entre los dos modos**: `annotated_in`, `scenarios` (`{ancla: [file:line]}`),
  `verification` (por ancla de una `quality`: `checks` con `{id, command, needs?}`, `sandboxes`,
  `verified_in`), `verified_by` con los objetos de check completos.
- **`--only <secciones>`** vale en los dos modos; la clave de identidad (`record` o `files`) siempre queda;
  una sección inválida para ese modo es error de uso.
- **Componentes de un archivo**: cuentan todos los que lo contienen, nunca solo el más específico — a
  diferencia de `component owners`, que elige uno.

## Decisiones

- **Por qué no hay comando `plan`**: `impact` ya devolvía, por record, con qué checks se verifica, qué
  sandbox pide y sus anotaciones; el modo archivos solo agrega resolver archivo → componentes → records y
  la unión. Un comando separado habría duplicado el motor.
- **`impact` informa, `check run`/`sandbox run` ejecutan** (tarea 13b): correr checks y sandboxes, y
  completar `{files}`, es mecánico y lo hace cumplir el binario, dando vuelta lo decidido en las tareas
  09/11 ("el binario nunca corre checks ni sandboxes"). El modo archivos no lleva `pending` (lo dice
  `status`) ni `warnings` (son avisos de `validate`, que corre antes).
- **Forma normalizada, checks dentro del record (N2)**: medido con Claude limpio en `orlab` (36 llamadas,
  Sonnet y Haiku). Sonnet acertó todo en cualquier forma; Haiku falló en la forma normalizada con checks
  por id (0.70) justo en "qué checks corro" — pregunta que de todos modos pasó a `check run`/`sandbox run`
  — y acertó 1.00 con los checks dentro del record. Agrupar por archivo acertó todo pero costó 3,6× tokens
  en la tarea grande.
- **`roadmap` (ex `plan --waves`) se diferió**: queda para después de las tareas 14 y 18 — ver tarea 21.

## Fuera de alcance / diferido

- `plan --waves` / hoja de ruta para sistemas grandes: pasó a ser el comando `roadmap`, tarea 21.
- Ejecutar o juzgar algo: eso quedó en `check run`/`sandbox run`, tarea 13b.
- Un único comando que sirva tanto para el modo record como para el modo archivos sin distinguirlos: se
  resolvió como dos modos del mismo comando, no como dos comandos.

## Criterios de aceptación

- [x] `openrecord impact --files <archivos...>` devuelve la forma JSON acordada, con tests que cubren un
      archivo, varios del mismo componente, y archivos de componentes distintos.
- [x] `files` cuenta todos los componentes que contienen cada archivo, sin caso especial para `root`.
- [x] `specs` incluye el salto a una `quality` cuyo `qualifies` nombra una spec ya incluida.
- [x] `--only` recorta secciones en los dos modos, con error de uso sobre un nombre inválido.
- [x] `docs/cli.md` explica la diferencia entre el modo record y el modo archivos, y cuándo usar cada uno.

## Commits

`9eadfbb` `e863ddb` `3ad8b0b`

## Escenario con IA

`e2e-ai/scenarios/13-impact-files.md` — PASS (2026-10-20, checked 2/2, judged 3/3, run
`20261020T174421Z-13-impact-files`, Claude Code 2.1.296 como orlab).
