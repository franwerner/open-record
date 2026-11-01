# 02 · Hash de integridad en registros JSON

| Campo | Valor |
| --- | --- |
| Depende de | — |
| Habilita | 06, 09, 11, 14, 15 |
| Costo | Bajo |
| Valor | Alto |
| Tipo | Formato / Binario |

## Contexto

Cada record en Markdown lleva `body-hash`, estampado por el binario y revisado por `validate`, que
detecta una edición hecha por fuera de la herramienta. `components.json` era el único JSON que el
binario escribía y no tenía ningún mecanismo equivalente. Esta tarea extiende la misma idea a todo
archivo JSON que el binario escribe dentro de `.openrecord/`.

## Qué se construyó

- **Campo `hash`** en todo JSON que el binario escribe (`components.json` primero; `actors.json`,
  `checks.json`, `sandbox.json`, `compliance/*.json` y los archivos de `runs/` lo heredan sin volver a
  diseñarlo), estampado en cada escritura.
- **Fórmula fija y documentada en un solo lugar** (`docs/decision-record.md#hash`, reutilizada por
  `internal/store/integrity.go`): SHA-256 del valor serializado con `json.MarshalIndent(value, "", "  ")`
  más un salto de línea final, con el campo `hash` vacío antes de calcular y presente (último, por
  convención visual) en el resultado final. Verificar un archivo en disco exige además que sea
  byte-por-byte lo que el binario habría escrito — no solo que el hash recalculado coincida — porque
  parsear descarta cualquier clave que el formato no declare.
- **Dos códigos de `validate`**: `json-hash-missing` (sin el campo) y `json-hash-mismatch` (no coincide, o
  el archivo no es byte-a-byte lo que el binario escribe), los dos error, para cualquier JSON cubierto.
- **Comandos de escritura se niegan en vez de re-estampar** sobre un archivo que no se puede confiar
  (`component add`/`component remove` fueron los primeros, y cada registro posterior repite el mismo
  patrón): el archivo queda intacto y la reparación es restaurarlo (`git checkout`), nunca parchear el
  campo a mano.
- **`runs/` queda fuera del recorrido de `validate` por defecto**: son datos de trabajo ignorados por git;
  el binario igual les estampa y puede revisarles el `hash`, pero desde los comandos propios de la
  ejecución (tarea 15), no desde `validate` general.
- **Comandos de solo lectura no se ven afectados**: `map`, `grep`, `search`, `diagram` siguen funcionando
  sobre un JSON con `hash` roto — leer no blanquea una manipulación.

## Decisiones

- **El binario es la única fuente que serializa**, así que no hace falta un canonicalizador JSON genérico:
  alcanza con que siempre escriba los mismos bytes para el mismo valor.
- **Un campo repetido en un archivo editado a mano se detecta aunque el hash "coincida" a simple vista**:
  por eso la verificación exige igualdad byte a byte, no solo igualdad de hash sobre lo parseado.
- **Sin comando de reparación**: la única forma de arreglar un `hash` roto es volver a escribir el archivo
  con el comando correspondiente, nunca parchear el campo.

## Fuera de alcance

- Firma criptográfica más allá de la detección de integridad: no hay amenaza de suplantación a resolver.
- Migraciones de formato de hash: no hace falta, no hay usuarios todavía.

## Criterios de aceptación

- [x] `components.json` incluye `hash` y `component add` / `component remove` lo recalculan en cada
      escritura.
- [x] `validate` reporta `json-hash-missing` y `json-hash-mismatch` sobre `components.json`, con tests que
      cubren ambos casos.
- [x] La fórmula del hash está documentada en un solo lugar (`docs/decision-record.md#hash`) que las
      tareas 06, 09, 11, 14 y 15 reutilizan sin reinventarla.
- [x] Queda explícito, en la documentación de `validate`, que los archivos bajo `runs/` no entran en el
      recorrido recursivo por defecto.

## Commits

`d71699f` `759c859`

## Escenario con IA

`e2e-ai/scenarios/02-integrity.md` — PASS (2026-10-13, checked 5/5, judged 3/3, run
`20261013T195436Z-02-integrity`, Claude Code 2.1.296 como orlab).
