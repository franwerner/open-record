# 20 · Worktree aislado por ejecución

| Campo | Valor |
| --- | --- |
| Depende de | 15 |
| Habilita | 18 |
| Costo | Medio |
| Valor | Medio |
| Tipo | Binario |

> Nació al definir la 15, que lo dejó afuera de su núcleo: es un mecanismo aparte. Todavía sin definir en
> detalle; lo de abajo es lo que la 15 proponía.

## Contexto

Sin worktree, un run trabaja sobre el mismo checkout donde la persona puede seguir editando: el store y
el código pueden cambiar bajo el run. La 15 lo cubre con la foto de `plan.json` y el desvío de
`run status`; un worktree propio lo evitaría de raíz.

## Lo que proponía la 15

- `run start` crea el worktree como carpeta hermana, dentro del home del usuario:
  `<padre-del-repo>/<repo>-worktrees/<id-de-ejecución>`, nunca en un directorio temporal del sistema.
- `runs/` vive en el checkout principal, no en el worktree: sobrevive a que el worktree se borre, y
  `run gaps` lo encuentra igual.
- Si la ejecución sale mal, se borra el worktree sin perder nada.
- Cada worktree necesita su propio índice de herramientas de exploración de código (no se reusa el del
  checkout principal).
- Puertos: si el checkout y el worktree levantan el mismo entorno a la vez, lo resuelve el proyecto; el
  sandbox no maneja puertos.
- Entre ejecuciones, en paralelo: cada una en su worktree y con su PR; dentro de una, las tareas en
  serie.

## Abierto

- Nombre de la rama del worktree, y quién la borra al cerrar.
- Cómo se comporta `run status` (desvío) cuando el run sí tiene worktree: el store del worktree es la
  foto, y el desvío es contra el checkout principal.
- Qué pasa con `compliance/` (versionado) escrito dentro del worktree hasta que se mergea.

## Notas de diseño originales (de `new/11-flujo-implementacion.md`)

Propuesta original, no decidida: el "índice propio de herramientas de exploración de código" de arriba se
refería puntualmente a **CodeGraph** — cada worktree necesita el suyo porque los archivos difieren del
checkout principal, y no se reusa el índice de este último.
