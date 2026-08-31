## Purpose

Gets a visitor an account they can come back to, while making sure the address they gave is one they can
actually receive mail at.

## Main flow

1. The visitor submits an email address and a password.
2. The system creates the account in a state that cannot yet sign in, and sends a verification link to
   the address.
3. The visitor opens the link, and the system marks the account usable and signs them in.

## Branches

- **[1] The password is shorter than the minimum** → nothing is created and the visitor is told the
  minimum length before submitting again.
- **[2] The address already has an account** → the same response is returned as for a new address, and
  a message is sent to the address saying someone tried to register it. Nothing in the response
  distinguishes the two cases.
- **[3] The link has expired** → the account stays unusable and the visitor is offered a new link.

## Edge cases

- The visitor opens the verification link twice: the second visit signs them in and does not fail.
- The visitor registers, never verifies, and registers again with the same address: a new link is sent
  and the previous one stops working.
- An account left unverified past the retention window is removed, and the address becomes available
  again.

## Errors facing the actor

- `password_too_short` — with the minimum length in the response.
- `link_expired` — the verification link is past its window; a new one can be requested.

## Scenarios

### Scenario: A verified visitor gets an account

- **GIVEN** an address with no account
- **WHEN** the visitor registers and opens the verification link
- **THEN** they are signed in and the account is usable

### Scenario: An unverified account cannot sign in

- **GIVEN** a visitor who registered and has not opened the link
- **WHEN** they try to sign in
- **THEN** they are refused and offered a new verification link

### Scenario: Registering a taken address reveals nothing

- **GIVEN** an address that already has an account
- **WHEN** someone registers with it
- **THEN** the response is identical to the one a new address produces, and the account holder is
  emailed about the attempt
