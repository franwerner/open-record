## Purpose

Tells an integrator exactly how much they may call the API and what they will see when they go past it,
so a client can be written to stay inside the limit rather than to discover it.

## Rule

A client may make **60 requests per minute** across the whole API, counted per API key on a rolling
sixty-second window.

The 61st request inside that window is refused with **429**, and the response carries a `Retry-After`
header in whole seconds saying when the window will have room again. A refused request does nothing: it
is not partially applied, and it is not counted against the next window.

The limit is the same for every key and for every endpoint. Reading and writing cost the same, and no
endpoint is exempt.

Requests refused for any other reason — an unknown key, a malformed body — still count against the
window. The budget covers what a client sends, not what it gets right.

## Scenarios

### Scenario: Inside the limit, nothing changes

- **GIVEN** a key that has made 59 requests in the last minute
- **WHEN** it makes one more
- **THEN** the request is served normally and no rate-limit error is returned

### Scenario: Past the limit, a 429 with a retry time

- **GIVEN** a key that has made 60 requests in the last minute
- **WHEN** it makes one more
- **THEN** the response is 429 and carries a `Retry-After` header with a whole number of seconds

### Scenario: A refused request has no effect

- **GIVEN** a key at its limit
- **WHEN** it submits a request that would have created an order
- **THEN** no order exists afterwards

### Scenario: Rejected requests still consume budget

- **GIVEN** a key that has sent 60 requests in the last minute, all with a malformed body
- **WHEN** it sends a valid request
- **THEN** the response is 429

### Scenario: The window rolls

- **GIVEN** a key that was refused with a `Retry-After` of 12 seconds
- **WHEN** it waits 12 seconds and retries
- **THEN** the request is served
