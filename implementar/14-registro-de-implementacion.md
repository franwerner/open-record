# 14 · Registro de cumplimiento (`compliance`, era "registro de implementación")

| Campo | Valor |
| --- | --- |
| Depende de | 02, 03 |
| Habilita | 13, 15, 18 |
| Costo | Medio |
| Valor | Alto |
| Tipo | Formato / Binario |

## Contexto

Nada en el store recordaba si un record ya estaba implementado en el código, cuándo se verificó por
última vez, o contra qué versión exacta. Sin esa memoria, cualquier automatización tendría que
reverificar todo el store completo cada vez. Esta tarea agrega ese registro.

## Qué se construyó

- **Nombre: `compliance`, no "implementation"**: registra si el código *cumple* cada record, también con
  eventos `adopted`, `verified` y `blocked` — "implementation" sugería solo el primer evento. Carpeta
  `.openrecord/compliance/<ruta-del-record>.json` (`.md` → `.json`), un archivo por record, en la misma
  ruta relativa que el record dentro del store. Comando `openrecord compliance`, con `status` y `log` (no
  `record`, que choca con el sustantivo `record` del resto de la CLI).
- **Un array de eventos por archivo**, cada uno con `body-hash` y `date` (`YYYY-MM-DD`) puestos por el
  binario, nunca por línea de comandos: `implemented`, `adopted` (el código ya cumplía, se registra sin
  tocar código), `verified` (re-chequeado sin cambio, sigue cumpliendo), `blocked`.
- **`verified-with`** (obligatorio en los tres eventos cumplidos): subconjunto no vacío de `checks`,
  `scenarios`, `sandbox`, `audit` — las cuatro partes de "terminado" de la tarea 18. `verified` significa
  "se aplicó la definición de terminado y se cumplió", no "pasaron los checks": un record sin checks es
  igual de verificable, por auditoría.
- **`blocked-by` estructurado**, reemplaza un `reason` de texto libre: lista no vacía de
  `{kind: pending|contradicts|missing, record?, note?}`. `pending`/`contradicts` exigen `record` existente;
  `missing` exige `note` y no lleva `record`. Sin campo `commit` (un commit no puede guardar su propio
  hash) ni `by` (no se registra quién lo escribió).
- **Estado calculado, nunca guardado**: `unknown` (sin archivo o sin eventos), `up_to_date` (el último
  evento cumplido coincide con el `body-hash` actual y no está pisado por un `blocked` posterior sobre el
  mismo hash), `blocked` (último evento `blocked` con el mismo hash), `outdated` (hay un evento cumplido
  pero el hash ya no coincide — el record se editó).
- **`possibly_stale`**: aviso sobre cualquier estado salvo `unknown`. Dispara con **cualquier cambio, sin
  umbral**, en un archivo cuyo componente **más específico** (`OwnerOf`, no "todos los que lo contienen"
  como en `impact --files`) es el del record. Basado en git, nunca en fechas: commiteado se mira con
  `git diff <referencia>..HEAD`, sin commitear con `git status`. La referencia es el último commit que
  tocó el **archivo de compliance** de ese record (nunca el del evento mismo, que no puede guardarse a sí
  mismo). Sin repositorio git, se reporta vacío siempre en vez de negarse.
- **`compliance status [--files]`** y **`compliance log`**, con la misma cadena archivo→componentes→
  records que `impact --files`. `log` no exige que `check run` haya pasado antes: la mayoría de los
  records no tiene checks, y lo que respalda el evento es cualquiera de las cuatro fuentes de
  `verified-with`.

## Decisiones

- **`possibly-stale` usa solo el componente más específico de cada archivo**, a diferencia de
  `impact --files` que cuenta todos los que lo contienen: con `root` = `.`, la regla de "todos los que lo
  contienen" marcaría todo `root` ante cualquier cambio en el repositorio, y el aviso dejaría de
  significar algo. `impact` responde "¿qué respeto en este archivo?" (ahí `root` cuenta); `possibly-stale`
  responde "¿de quién es el código que cambió?" (ahí no debería). Lo que se pierde — una violación de
  `root` desde un subcomponente — lo atrapan los checks de `root` con `check run`.
- **Sin atajo masivo para un proyecto existente** (`adopt --all` se descartó): sería el falso positivo en
  escala. Arranca todo `unknown`, que `compliance status` muestra, y se resuelve de a tandas con el flujo
  de la tarea 18 — documentado como estado normal, no un backlog de fallas.
- **`compliance` no reemplaza la verificación real**: es un atajo para no reverificar lo ya sabido; la
  verdad sigue saliendo de comparar el código con el record.

## Fuera de alcance

- Guardar el commit en el propio evento: un commit no puede guardar su propio hash; git ya responde esa
  pregunta cuando hace falta.
- Un umbral de tiempo o de volumen de cambio para `possibly_stale`: cualquier cambio lo dispara, sin
  excepción.
- Borrar automáticamente un registro huérfano (`compliance-orphan`): se reporta como advertencia; borrarlo
  queda para quien mantiene el store.

## Criterios de aceptación

- [x] El binario escribe y lee `.openrecord/compliance/<ruta>.json`, con tests que cubren los cuatro tipos
      de evento.
- [x] El cálculo de estado (`up_to_date`, `outdated`, `blocked`, `unknown`) tiene un test por cada estado,
      incluido el caso sin archivo de registro.
- [x] `possibly_stale` se calcula contra git (commiteado y sin commitear) y tiene un test que confirma que
      desaparece después de un evento `verified` posterior al cambio.
- [x] Un evento `blocked` sin `blocked-by` es rechazado en la escritura, con un test.
- [x] La documentación explica por qué el registro no reemplaza la verificación real y qué significa
      "desconocido" frente a "sin implementar".

## Commits

`0b6a2da` `6c44ad9`

## Escenario con IA

`e2e-ai/scenarios/14-compliance.md` — PASS (2026-10-20, checked 4/4, judged 2/2, run
`20261020T222621Z-14-compliance`, Claude Code 2.1.296 como orlab).
