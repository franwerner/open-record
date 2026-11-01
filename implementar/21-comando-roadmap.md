# 21 · Comando `roadmap`

| Campo | Valor |
| --- | --- |
| Depende de | 05, 14 |
| Habilita | 18 |
| Costo | Medio |
| Valor | Alto |
| Tipo | Binario |

> Era `plan --waves` (tarea 13), diferido hasta tener la 14. Revivido al definir la 15. Sin definir en
> detalle.

## Contexto

Quien usa openrecord a través de una IA y no sabe por dónde empezar le dice "implementá"; la skill
`implement` (18) necesita proponer un alcance con un orden que no dependa de la intuición. Este comando
da ese orden.

## Lo ya decidido

- **Comando aparte, no un flag de otro** (no recibe archivos ni un record; devuelve otra forma).
- **El orden sale solo de las relaciones:** capas en orden topológico por `depends-on`, `serves` y
  `qualifies` (una decisión antes que lo que depende de ella, antes que la spec que sirve; una spec antes
  que su quality). Sin olas con nombre ("esqueleto", "núcleo"…): el store no marca qué es esqueleto.
- **Saltea lo que ya está `up_to_date` en `compliance`** (tarea 14).

```
openrecord roadmap
→ ola 0: decisions/root/data/postgres.md, decisions/root/structure/go.md
  ola 1: decisions/api/data/pgx.md
  ola 2: specs/flow/checkout/place-an-order.md
  ola 3: specs/quality/checkout.md
```

## Abierto

- **`run start` sin `--scope`** (idea de la 15): que el alcance lo calcule el binario a partir de lo que falta
  implementar (la próxima ola del roadmap), con un límite de tamaño. La 15 deja el código preparado.

- Qué pasa con `outdated`, `possibly_stale` y `blocked` (¿entran, y en qué ola?).
- Forma de la salida (records sueltos o agrupados por carpeta, como `specs/flow/checkout/*`).
- Records sin ninguna relación: ¿ola 0?
