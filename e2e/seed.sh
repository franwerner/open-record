#!/usr/bin/env bash
# Rebuilds e2e/project/.openrecord from scratch, using the openrecord in this
# working tree.
#
# The fixture is checked in, but it is never hand-edited: it is whatever these
# commands produce. That is the point — a fixture somebody wrote by hand can
# drift into a shape the tool would never write, and then every test that reads
# it is testing a file rather than the tool.
#
#   ./e2e/seed.sh          rebuild the fixture
#   ./e2e/seed.sh --check  rebuild into a temp dir and diff; non-zero if it drifted
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(dirname "$here")"
bodies="$here/bodies"

check=0
[ "${1:-}" = "--check" ] && check=1

binary="$(mktemp -d)/openrecord"
go build -o "$binary" "$root/cmd/openrecord"

if [ "$check" -eq 1 ]; then
  target="$(mktemp -d)/project"
  mkdir -p "$target"
  cp -r "$here/project/src" "$target/"
else
  target="$here/project"
  rm -rf "$target/.openrecord"
fi

or() { "$binary" --repo "$target" "$@" >/dev/null; }

# ---------------------------------------------------------------- components --
or component add api --path src/api \
  --title "API" \
  --description "The HTTP surface. Descend here if you touch an endpoint, a request or response shape, or an error a client is shown."

or component add web --path src/web \
  --title "Web app" \
  --description "The browser application. Descend here if you touch a screen, what it holds while open, or what a visitor sees while waiting."

or component add cli --path src/cli \
  --title "Operator CLI" \
  --description "The command line operators run against a deployment. Descend here if you touch a command, its flags, its output or its exit status."

or component add root --path . \
  --title "Repository" \
  --description "Build, configuration, deployment and the boundary of the system as a whole. Descend here if you touch CI, environment configuration, or something true of every surface at once."

# -------------------------------------------------------------------- levels --
or level add decisions/api/security
or level add decisions/api/security/rate-limits \
  --title "Rate limits" \
  --description "Where request limits are applied and how a client's budget is computed. Descend here before adding any check that counts requests."
or level add decisions/api/runtime
or level add decisions/api/contracts
or level add decisions/web/structure
or level add decisions/cli/contracts
or level add decisions/root/delivery
or level add decisions/root/security

# The four spec types describe themselves: the binary ships their prose, the
# same way it ships a concern's. A subgroup does not — what a cluster of records
# shares is a judgement about those records.
or level add specs/flow
or level add specs/rule
or level add specs/lifecycle
or level add specs/process
or level add specs/flow/checkout --title "Checkout" \
  --description "Everything between a basket and a paid order: the totals a shopper is shown, what is charged, and what happens when a payment does not go through."

# ----------------------------------------------------------------- decisions --
or record write decisions/api/security/rate-limits/at-the-gateway.md \
  --title "Rate limiting is applied at the gateway" \
  --description "Request limits are applied at the gateway and computed per API key across the whole surface, never inside a handler." \
  --status accepted --body-file "$bodies/api-rate-limits-gateway.md"

or record write decisions/api/security/rate-limits/per-key-quotas.md \
  --title "Whether a key can have its own quota" \
  --description "Whether an individual API key can carry a quota of its own is deliberately undecided; the single shared limit governs until someone settles it." \
  --status pending --body-file "$bodies/api-rate-limits-per-key-quotas.md"

or record write decisions/api/runtime/error-translation.md \
  --title "Business errors are their own hierarchy" \
  --description "Business errors are modelled as their own hierarchy, and a provider's errors are translated at the adapter boundary so no foreign error type travels inward." \
  --status accepted --body-file "$bodies/api-error-translation.md"

or record write decisions/api/contracts/versioning.md \
  --title "The version is in the URL, and a version is additive" \
  --description "The API version is carried in the URL; inside a version only additive change ships, and a consumer keeps working until that version is retired." \
  --status accepted --body-file "$bodies/api-versioning.md"

or record write decisions/web/structure/state-ownership.md \
  --title "Server data is never held as local state" \
  --description "Server data is fetched and invalidated by the mutation that changed it, never written into local state; a screen owns only what closing it would lose." \
  --status accepted --body-file "$bodies/web-state-ownership.md"

or record write decisions/cli/contracts/output-contract.md \
  --title "Readable by default, JSON on request" \
  --description "Commands print for a person by default and switch wholly to JSON with a flag; exit status separates success, failure and a malformed invocation." \
  --status accepted --body-file "$bodies/cli-output-contract.md"

or record write decisions/root/delivery/testing-strategy.md \
  --title "Test at the boundary the change would be noticed at" \
  --description "Behaviour is tested through the surface that offers it with providers stubbed at the adapter boundary; wiring is deliberately untested, and a test asserts an outcome, never a call." \
  --status accepted --body-file "$bodies/root-testing-strategy.md"

or record write decisions/root/security/no-auth-in-the-service.md \
  --title "The service authenticates nothing itself" \
  --description "Identity is established at the gateway and taken as given inward; the service performs no authentication of its own, and this absence is decided rather than forgotten." \
  --status accepted --body-file "$bodies/root-no-auth-in-the-service.md"

# --------------------------------------------------------------------- specs --
or record write specs/flow/checkout/place-an-order.md \
  --title "Place an order" \
  --description "A shopper turns a basket into a paid order, seeing the total before it is charged; a decline leaves the basket untouched and a repeated submission charges once." \
  --status accepted --components api --components web \
  --body-file "$bodies/spec-place-an-order.md"

or record write specs/flow/sign-up.md \
  --title "Sign up" \
  --description "A visitor registers with an email and password; the account cannot sign in until the emailed link is opened, and a taken address is indistinguishable from a free one." \
  --status accepted --components api --components web \
  --body-file "$bodies/spec-sign-up.md"

or record write specs/rule/usage-limits.md \
  --title "Usage limits" \
  --description "60 requests per minute per API key across the whole surface; the next one is refused with 429 and a Retry-After, and refused requests still consume budget." \
  --status accepted --components api \
  --body-file "$bodies/spec-usage-limits.md"

or record write specs/lifecycle/order.md \
  --title "Order" \
  --description "An order runs from awaiting payment to delivered, and can leave for abandoned, refunded, lost in transit or returned; cancellation is possible until the warehouse picks it." \
  --status accepted --components api --components web --components cli \
  --body-file "$bodies/spec-order-lifecycle.md"

or record write specs/process/payment-webhook.md \
  --title "Payment webhook" \
  --description "A signed provider event moves an order to match what the provider did; repeated deliveries are harmless, unsigned ones are refused, and an unreachable transition is left for a person." \
  --status accepted --components api \
  --body-file "$bodies/spec-payment-webhook.md"

# --------------------------------------------------------------------- check --
"$binary" --repo "$target" validate >/dev/null

if [ "$check" -eq 1 ]; then
  if ! diff -r "$here/project/.openrecord" "$target/.openrecord"; then
    printf '\ne2e/project/.openrecord has drifted from what seed.sh produces.\n' >&2
    printf 'Re-run ./e2e/seed.sh and commit the result.\n' >&2
    exit 1
  fi
  printf 'fixture matches seed.sh\n'
else
  printf 'fixture rebuilt at e2e/project/.openrecord\n'
fi
