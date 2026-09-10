#!/usr/bin/env bash
# Full end-to-end walk, run as a Unix user who has never seen openrecord.
#
# Nothing is pre-installed and nothing is pre-configured: the published
# installer, the published qmd, the skills as they ship.
#
# Every check asserts a VALUE, not the presence of one. The previous run of this
# script went green on a record whose body was a stale file from another user —
# it only checked that a search returned something, never that what came back
# was right.
set -uo pipefail

set -a; . "$HOME/creds.env"; set +a   # lo deja el arnés; ver TESTING.md
export PATH="$HOME/.local/bin:$HOME/.npm-global/bin:$PATH"
W="$HOME/work"; ok=0; bad=0

step()  { printf '\n\033[1;36m━━ %s\033[0m\n' "$*"; }
note()  { printf '    \033[2m%s\033[0m\n' "$*"; }
check() { # check <got> <want> <label>
  if [ "$1" = "$2" ]; then printf '  \033[32m✓\033[0m %s\n' "$3"; ok=$((ok+1))
  else printf '  \033[31m✗\033[0m %s\n      esperaba: %s\n      dio:      %s\n' "$3" "$2" "$1"; bad=$((bad+1)); fi; }
# exit code of a command, without a pipe (a pipe + `grep -q` yields SIGPIPE
# under pipefail, which is what produced a phantom 141 last time)
rc() { "$@" >/dev/null 2>&1; echo $?; }

# Guardia de parseo JSON, en dos entradas sobre un mismo cuerpo. Ninguna de las
# dos toca ok/bad — check "$(...)" corre en una subshell, así que un contador
# tocado ahí adentro se pierde al volver. En cambio se reporta por valor: la
# diagnosis va a STDERR (y ya está impresa cuando check corre, porque la
# substitución de comandos termina antes) y $JBAD sale por STDOUT, que nunca
# coincide con un valor esperado — así el check que lo envuelve es el que falla.
JBAD='<sin json — el error va arriba>'   # sentinela: nunca coincide con un valor esperado
JERR="$HOME/.jerr"                        # el stderr de la última invocación de jval

jstr() { # jstr <expr> <texto>: parsea <texto> como JSON en `d` y corre <expr>.
  local expr="$1" text="$2" out
  if out="$(printf '%s' "$text" | python3 -c "
import json, sys
d = json.load(sys.stdin)
$expr
" 2>/dev/null)"; then
    printf '%s' "$out"
  else
    printf '%s\n' "$text" >&2
    printf '%s' "$JBAD"
  fi
}

jval() { # jval <expr> <cmd...>: corre <cmd> con stdout capturado y stderr en $JERR.
  local expr="$1"; shift
  local out code
  out="$("$@" 2>"$JERR")"; code=$?
  if [ "$code" -ne 0 ]; then
    printf 'exit %s\n%s\n' "$code" "$out" >&2
    cat "$JERR" >&2
    printf '%s' "$JBAD"
  else
    jstr "$expr" "$out"
  fi
}

step "0. Punto de partida"
note "usuario: $(id -un)  ·  HOME: $HOME"
if [ "${BUILD_LOCAL:-no}" = "yes" ]; then
  note "openrecord: colocado por el arnés desde el working tree"
else
  check "$(command -v openrecord || echo nada)" "nada" "openrecord no está instalado"
fi
# En esta máquina hay un qmd system-wide (/usr/local/bin/qmd, symlink a un
# checkout de matecito-ai), así que ningún usuario arranca sin qmd en el PATH.
# Lo que importa para la prueba es que ESTE usuario no tenga uno propio.
note "qmd en el PATH del sistema: $(command -v qmd || echo ninguno)"
check "$(ls "$HOME"/.npm-global/bin/qmd "$HOME"/.local/bin/qmd 2>/dev/null | wc -l)" "0" "el usuario no tiene un qmd propio"
check "$(ls -d "$HOME/.config/qmd" 2>/dev/null || echo nada)" "nada" "no hay config de qmd"

if [ "${BUILD_LOCAL:-no}" = "yes" ]; then
  # El instalador del working tree, contra el release que el arnés armó al lado:
  # BASE_URL y VERSION vienen del entorno, y son lo único que cambia respecto de
  # lo que corre un usuario.
  step "1. Instalación del working tree, por su propio instalador"
  WITH_QMD=no bash "$HOME/install.sh" 2>&1 | sed 's/^/    /'
  hash -r
  check "$(rc command -v openrecord)" "0" "el instalador dejó el binario en el PATH"
  note "$(openrecord version | tr -d '\n ')"
  check "$(jval 'print(d["commit"])' openrecord version)" \
        "${EXPECT_COMMIT}" "corre el commit que está en master ahora"
else
  step "1. Instalación por la vía publicada"
  curl -fsSL https://raw.githubusercontent.com/franwerner/open-record/master/scripts/install.sh | WITH_QMD=no bash 2>&1 | sed 's/^/    /'
  hash -r
  check "$(jval 'print(d["version"])' openrecord version)" "0.2.0" "instaló v0.2.0"
fi
check "$(jval 'print(d["store_format"])' openrecord version)" "1" "formato de store 1"

step "2. qmd, por la ruta que ofrece openrecord"
npm config set prefix "$HOME/.npm-global" >/dev/null 2>&1
openrecord qmd install >/dev/null 2>&1; hash -r
st() { jval "print(d[\"$1\"])" openrecord qmd status; }
check "$(st installed)" "True" "installed"
check "$(st usable)"    "True" "usable — corre, no solo está"
check "$(jval 'print(d["pinned_version"] in d["version"])' openrecord qmd status)" "True" "es la versión que openrecord fija"

step "3. Proyecto y superficies"
mkdir -p "$W/src/api" "$W/src/worker" && cd "$W"
cat > src/api/handlers.py <<'PY'
# Every write endpoint requires an Idempotency-Key header and refuses without
# one: the client retries on any network hiccup, and a retried charge landing
# twice is the one bug this service must never have.
IDEMPOTENCY_HEADER = "Idempotency-Key"
PY
cat > src/worker/jobs.py <<'PY'
# Jobs run in-process on a schedule rather than through a queue. There is one
# instance and the work is idempotent, so a queue would add an operational
# component to buy a guarantee we already have.
SCHEDULE_SECONDS = 60
PY
git init -q . && git add -A && git -c user.email=a@b -c user.name=t commit -q -m init
openrecord component add api    --path src/api    --title "API"    --description "The HTTP surface. Descend here if you touch an endpoint or what a client is told." >/dev/null
openrecord component add worker --path src/worker --title "Worker" --description "Scheduled work. Descend here if you touch what runs on its own." >/dev/null
check "$(jval 'print(",".join(e["path"] for e in d["entries"]))' openrecord map --for decisions)" \
      "decisions/api,decisions/worker" "las dos superficies quedaron declaradas"

step "4. Niveles: de dónde sale la prosa de cada uno"
src() { jval 'print(d["source"])' openrecord level add "$1" ${2:+--title "$2"} ${3:+--description "$3"}; }
check "$(src decisions/api/security)" "catalogue" "una concern la trae el catálogo"
check "$(src specs/flow)"             "shipped"   "un tipo de spec lo trae el binario"
check "$(src specs/rule)"             "shipped"   "y el otro también"
check "$(rc openrecord level add specs/flow/checkout)" "1" "un subgrupo sin prosa se rechaza"
check "$(jstr 'print("subgroup" in d["message"])' "$(openrecord level add specs/flow/checkout 2>&1)")" \
      "True" "y el rechazo habla de subgrupos, no del catálogo"

step "5. Escribir, y verificar lo que quedó escrito"
# El cuerpo va en $HOME. En /tmp un archivo de otro usuario no se puede pisar, y
# entonces se escribe un record con contenido ajeno sin que nada avise.
cat > "$HOME/body.md" <<'BODY'
## Context

The client retries on any network hiccup, and it cannot tell a request that never arrived from one
whose response was lost. Without a key, the second attempt is a second charge.

## Decision

Every write endpoint requires an Idempotency-Key header and refuses the request without one, before it
reaches anything that persists.

## Alternatives

- **Optional key, deduplicate when present.** Rejected: the clients that most need it are the ones
  that would forget it, so the protection would be absent exactly where it matters.
- **Deduplicate on the payload.** Rejected: two legitimate identical charges are a real thing, and
  nothing in the payload distinguishes them from a retry.

## Consequences

Every caller has to generate a key, a real burden on integrators and the reason it is documented before
anything else. A request without one fails loudly rather than being silently deduplicated.
BODY
openrecord record write decisions/api/security/idempotency-key-required.md \
  --title "Writes require an idempotency key" \
  --description "Every write endpoint requires an Idempotency-Key header and refuses the request without one." \
  --status accepted --body-file "$HOME/body.md" >/dev/null
# El cuerpo en disco tiene que ser EL QUE MANDÉ, no cualquiera que se haya leído.
written=".openrecord/decisions/api/security/idempotency-key-required.md"
check "$(python3 - <<PY
body=open("$written").read().split("---",2)[2].strip()
want=open("$HOME/body.md").read().strip()
print(body==want)
PY
)" "True" "el cuerpo en disco es exactamente el que se pasó"
check "$(grep -c '^title: Writes require an idempotency key$' "$written")" "1" "el título quedó en el frontmatter"

# El worker también necesita un record propio — es la superficie que responde
# "qué corre trabajo que nadie disparó" en el paso 11. openrecord no deja
# escribir un decision directo bajo un componente: hace falta una concern.
openrecord level add decisions/worker/runtime >/dev/null
cat > "$HOME/worker.md" <<'BODY'
## Context

Work that no request triggers still has to run somehow: nothing calls this service to ask for it, so
something in it has to run that work on its own.

## Decision

Jobs run in-process on a schedule, inside the same process that serves requests, rather than through a
separate queue or a dedicated worker fleet.

## Alternatives

- **A message queue with dedicated workers.** Rejected: it buys at-least-once delivery under a crash, a
  guarantee this service already has for free — there is one instance and every job is idempotent.
- **A cron job outside the process, calling back in.** Rejected: it adds a second deployable that has to
  stay in sync with the code it schedules, for no guarantee the in-process scheduler doesn't already give.

## Consequences

A single instance is the whole story: if it is down, scheduled work does not run until it comes back.
Idempotency is what makes an in-process retry safe, so any job added later has to keep that property.
BODY
openrecord record write decisions/worker/runtime/scheduled-in-process-jobs.md \
  --title "Jobs run in-process on a schedule, not through a queue" \
  --description "Jobs run in-process on a schedule rather than through a queue; there is one instance and the work is idempotent." \
  --status accepted --body-file "$HOME/worker.md" >/dev/null

cat > "$HOME/spec.md" <<'BODY'
## Purpose

Tells an integrator what happens when the same write arrives twice, so a client can be written to retry
safely rather than to discover the rule by charging somebody twice.

## Rule

Every write endpoint requires an `Idempotency-Key` header. A request without one is refused with **400**
and the code `idempotency_key_required`, before anything is persisted. A repeat of a key already seen
returns the original outcome and creates nothing new.

## Scenarios

### Scenario: A write without the header is refused

- **GIVEN** a caller sending a write request
- **WHEN** the request carries no `Idempotency-Key` header
- **THEN** the response is 400 with code `idempotency_key_required` and nothing is persisted

### Scenario: The same key twice charges once

- **GIVEN** a write that already succeeded with a given key
- **WHEN** the identical request is sent again with that same key
- **THEN** the original outcome is returned and no second charge exists
BODY
openrecord record write specs/rule/idempotent-writes.md \
  --title "Idempotent writes" \
  --description "Writes require an Idempotency-Key; a request without one is refused with 400, and a repeated key returns the original outcome without charging again." \
  --status accepted --components api --body-file "$HOME/spec.md" >/dev/null
check "$(jval 'e=d["entries"][0];print(",".join(e["components"]))' openrecord map --for specs/rule)" \
      "api" "el spec declara la superficie que toca"
check "$(rc openrecord validate)" "0" "validate limpio"

step "6. Lo que tiene que ser rechazado"
printf '## Context\n\nx\n\n## Decision\n\nx\n' > "$HOME/partial.md"
check "$(rc openrecord record write decisions/api/security/incompleto.md --title T --description D --status accepted --body-file "$HOME/partial.md")" "1" "un cuerpo sin las cuatro secciones"
check "$(ls .openrecord/decisions/api/security/ | grep -c incompleto)" "0" "y no dejó archivo"
check "$(rc openrecord record write specs/rule/sin-comp.md --title T --description D --status accepted --body-file "$HOME/spec.md")" "1" "un spec sin --components"
check "$(rc openrecord record write specs/rule/mala.md --title T --description D --status accepted --components inexistente --body-file "$HOME/spec.md")" "1" "un spec con una superficie no declarada"
check "$(rc openrecord record write decisions/api/security/est.md --title T --description D --status proposed --body-file "$HOME/body.md")" "1" "un status que no existe"
before="$(sha256sum "$written" | cut -d' ' -f1)"
printf '## Rule\n\nno va acá\n' > "$HOME/mal.md"
check "$(rc openrecord record edit "$written" --section '## Decision' --body-file "$HOME/mal.md")" "1" "un edit que rompe el record"
check "$(sha256sum "$written" | cut -d' ' -f1)" "$before" "y el archivo quedó byte a byte igual"
check "$(jstr 'print("written" in d, "edited" in d)' "$(openrecord record edit "$written" --section '## Decision' --body-file "$HOME/mal.md" 2>&1)")" \
      "False True" "el fallo se reporta bajo 'edited', no bajo 'written'"

step "7. Navegación y contrato"
check "$(rc openrecord map --for specs/process)"        "0" "un tipo de spec vacío se puede abrir"
check "$(rc openrecord map --for decisions/nope)"       "1" "map rechaza una coordenada inexistente"
check "$(rc openrecord validate --for decisions/nope)"  "1" "y validate coincide"
check "$(jstr 'print("is a record" in d["message"])' "$(openrecord map --for "$written" 2>&1)")" \
      "True" "una coordenada que nombra un record lo dice"
h="$(openrecord record write --help)"
# Aparece dos veces y está bien: en la linea de Usage y en la lista de Flags.
check "$(printf '%s' "$h" | grep -A20 '^Flags:' | grep -c -- '--components')" "1" "el help lista --components entre sus flags"
check "$(printf '%s' "$(openrecord skills --help)" | grep -A20 '^Flags:' | grep -c -- '--dry-run')" "1" "y el de skills lista --dry-run"
o="$(openrecord component owners src/api/handlers.py 2>&1)"
check "$(jstr 'print(d["owner"],",".join(d["specs"]))' "$o")" \
      "api specs/rule/idempotent-writes.md" "owners devuelve la superficie y los specs que la nombran"
g="$(openrecord grep 'Idempotency-Key' --for decisions 2>&1)"
check "$(jstr 'm=d["matches"];print(len(m),len({x["path"] for x in m}))' "$g")" \
      "1 1" "grep devuelve una entrada por record, no por línea"
check "$(jstr 'print(d["matches"][0]["hits"]>1)' "$g")" "True" "y cuenta las líneas que coincidieron"

step "8. Skills y emit"
openrecord skills --emit .claude/skills/ --with-qmd >/dev/null
check "$(ls .claude/skills | sort | tr '\n' ' ')" \
      "openrecord-bootstrap openrecord-capture openrecord-consult openrecord-mine openrecord-setup-search " "las cinco skills, todas con prefijo"
fp() { find .claude/skills -type f -exec sha256sum {} + | sort | sha256sum; }
b="$(fp)"; openrecord skills --emit .claude/skills/ --with-qmd --dry-run >/dev/null
check "$(fp)" "$b" "--dry-run no tocó nada"

# Las skills viajan dentro del binario, así que esto es lo único que dice si los
# arreglos sin publicar llegaron de verdad al usuario.
m=.claude/skills/openrecord-mine/SKILL.md
check "$(grep -c 'The anchor is for the gate, never for the record' $m)" "1" "mine: el ancla es para la gate, no para el record"
check "$(grep -c 'least of all in its `description`' $m)"                "1" "mine: la regla nombra la description"
check "$(grep -c 'A section with nothing in it is deleted, not filled' $m)" "1" "mine: una sección vacía se borra"
check "$(grep -c 'internal/' $m)" "0" "mine: no nombra rutas que solo existen en el repo de openrecord"
s=.claude/skills/openrecord-setup-search/SKILL.md
check "$(grep -c 'Which provider is the user.s call, not yours' $s)" "1" "setup-search: el proveedor lo elige el usuario"
check "$([ "$(grep -c 'qmd collection add' $s)" -ge 1 ] && echo si || echo no)" "si" "setup-search: nombra el comando que registra"

step "9. Diagramas"
cat > "$HOME/life.md" <<'BODY'
## Purpose

Says which states a charge can be in and what moves it between them.

## States and transitions

- awaiting confirmation → confirmed (the provider accepts the authorisation)
- awaiting confirmation → abandoned (the caller never confirms within the window)
- confirmed → refunded (the payer disputes it and the dispute is upheld)

## Scenarios

### Scenario: An unconfirmed charge is abandoned

- **GIVEN** a charge awaiting confirmation
- **WHEN** the window passes with no confirmation
- **THEN** the charge is abandoned and nothing is captured
BODY
openrecord level add specs/lifecycle >/dev/null
openrecord record write specs/lifecycle/charge.md --title "Charge" \
  --description "A charge runs from awaiting confirmation to confirmed, and can leave for abandoned or refunded." \
  --status accepted --components api --body-file "$HOME/life.md" >/dev/null
d="$(openrecord diagram specs/lifecycle/charge.md)"
check "$(printf '%s' "$d" | grep -c 'state "awaiting confirmation" as awaiting_confirmation')" "1" "el alias de un estado multi-palabra sale bien"
check "$(printf '%s' "$d" | grep -c '""')" "0" "sin comillas duplicadas — mermaid las rechaza"
check "$(printf '%s' "$d" | grep -c 'awaiting_confirmation --> confirmed: the provider accepts the authorisation')" "1" "la transición conserva su disparador entero"
check "$(rc openrecord diagram specs/rule/idempotent-writes.md)" "1" "un rule no tiene diagrama"

step "10. Búsqueda semántica, desde un índice inexistente"
mkdir -p "$HOME/.config/qmd"
printf 'models:\n  embed: openai/text-embedding-3-small\n  generate: openai/gpt-4o-mini\n' > "$HOME/.config/qmd/index.yml"
check "$(qmd doctor 2>&1 | grep -c 'search provider: not exercised')" "1" "en un índice vacío el doctor no afirma nada del proveedor"
collections_needed="$(jval 'print(" ".join(d["collections_needed"]))' openrecord qmd status)"
check "$([ "$collections_needed" = "$JBAD" ] && echo "$collections_needed" || echo ok)" "ok" "el status devolvió la lista de colecciones a registrar"
for c in $collections_needed; do
  sub=$(echo "$c" | sed 's/^work-//; s/decisions-/decisions\//')
  qmd collection add "$PWD/.openrecord/$sub" --name "$c" --mask '**/*.md' >/dev/null 2>&1
done
check "$(qmd collection list 2>/dev/null | grep -c '^work-')" "3" "tres colecciones registradas"
if ! embed_out="$(qmd embed 2>&1)"; then
  printf '%s\n' "$embed_out" >&2
fi
check "$(qmd doctor 2>&1 | grep -c 'search provider: answered every call')" "1" "después de embeber, el proveedor respondió"

step "11. Las búsquedas: ¿devuelven lo correcto?"
Q='what keeps a retried request from charging twice'
note "pregunta: $Q"
check "$(jval 'print(len(d["matches"]))' openrecord grep 'retried' --for decisions)" "0" "el literal no encuentra nada — no está esa palabra"
top="$(jval 'print(d[0]["file"] if d else "nada")' qmd vsearch "$Q" -c work-decisions-api -c work-specs --format json)"
note "primer resultado semántico: $top"
check "$(printf '%s' "$top" | grep -c 'idempotency-key-required\|idempotent-writes')" "1" "y el semántico devuelve EL record correcto"
Q2='why a queue was rejected for background work'
top2="$(jval 'print(d[0]["file"] if d else "nada")' qmd vsearch "$Q2" -c work-decisions-worker --format json)"
note "pregunta: $Q2  →  $top2"
check "$top2" "qmd://work-decisions-worker/runtime/scheduled-in-process-jobs.md" "una consulta acotada a una superficie devuelve la suya"

step "12. Proveedor caído: no puede parecer un resultado vacío"
bad_key() { QMD_OPENAI_API_KEY=sk-or-v1-INVALIDA "$@" >/dev/null 2>&1; echo $?; }
QMD_OPENAI_API_KEY=sk-or-v1-INVALIDA qmd vsearch "$Q" -c work-specs 2>&1 | tail -2 | sed 's/^/    /'
check "$(bad_key qmd vsearch "$Q" -c work-specs)" "1" "la búsqueda sale 1"
check "$(bad_key qmd vsearch "$Q" -c work-specs --format json)" "1" "y también con --format json"
check "$(bad_key qmd doctor)" "1" "qmd doctor sale 1"
check "$(rc qmd search 'zzzznoexistezzzz' -c work-specs)" "0" "una búsqueda sana que no encuentra nada sale 0"

step "13. Estado final"
check "$(rc openrecord validate)" "0" "el store sigue válido"
note "records: $(find .openrecord -name '*.md' -not -name INDEX.md | wc -l)  ·  componentes: $(python3 -c 'import json;print(len(json.load(open(".openrecord/components.json"))["components"]))')"

printf '\n\033[1m━━ Resultado: %s ok, %s fallas\033[0m\n' "$ok" "$bad"
[ "$bad" -eq 0 ] && printf '\033[32m   Todo verde.\033[0m\n' || printf '\033[31m   Hay fallas.\033[0m\n'
exit $(( bad > 0 ))
