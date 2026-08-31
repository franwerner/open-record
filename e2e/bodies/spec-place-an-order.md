## Purpose

Lets a shopper turn the contents of their basket into an order that the warehouse can act on, and tells
them what they owe before they commit to it.

## Main flow

1. The shopper opens the checkout with a basket that has at least one item, and the system shows the
   items, the total, and where it will be delivered.
2. The shopper confirms the delivery address, and the system recalculates the total to include delivery
   and any tax that the destination attracts.
3. The shopper submits payment details, and the system asks the payment provider to authorise the
   amount shown — never an amount the shopper has not seen.
4. The system creates the order, empties the basket, and shows an order reference the shopper can quote
   when asking about it later.
5. The system sends a confirmation to the shopper's email address with the same reference and the
   delivery estimate.

## Branches

- **[1] The basket is empty** → the shopper is shown the basket rather than the checkout, with a note
  that there is nothing to pay for yet.
- **[2] The destination is one the shop does not deliver to** → the total is not recalculated and the
  shopper is told which destinations are available, before any payment detail is asked for.
- **[3] The provider declines the authorisation** → no order is created, the basket keeps its contents,
  and the shopper is told the payment was declined and can try another method.
- **[3] The total changed between being shown and being submitted** → the authorisation is not
  attempted, and the shopper is shown the new total and asked to confirm it again.
- **[5] The confirmation email cannot be sent** → the order still exists and is still shown to the
  shopper; only the email is retried, and its failure never undoes a paid order.

## Edge cases

- An item goes out of stock while the shopper is on the checkout: the order is created for what is
  available, the unavailable line is dropped, and the shopper is told which line was dropped before
  paying.
- The shopper submits the same checkout twice — a double click, or a reloaded page: one order is
  created, and the second submission returns the first order's reference rather than charging again.
- The shopper leaves and comes back a day later: the basket survives, and the total is recalculated
  from current prices, with any change shown before payment.

## Errors facing the actor

- `basket_empty` — there is nothing to check out.
- `destination_unsupported` — the shop does not deliver there, and the response lists what it does
  deliver to.
- `payment_declined` — the provider refused the authorisation; the message is the shopper's to act on
  and never carries the provider's own wording.
- `total_changed` — the amount moved between being shown and being submitted, and the new amount is in
  the response.

## Scenarios

### Scenario: A basket becomes an order

- **GIVEN** a shopper with two items in their basket and a deliverable address
- **WHEN** they submit valid payment details for the total they were shown
- **THEN** an order exists with both items, the basket is empty, and they are shown an order reference

### Scenario: A declined payment leaves the basket alone

- **GIVEN** a shopper at the payment step
- **WHEN** the provider declines the authorisation
- **THEN** no order exists, the basket still holds both items, and they are told the payment was
  declined

### Scenario: A price change is shown before it is charged

- **GIVEN** a shopper who was shown a total of 40
- **WHEN** the price of one item rises before they submit
- **THEN** nothing is authorised, and they are shown the new total and asked to confirm it

### Scenario: Submitting twice charges once

- **GIVEN** a shopper who has just submitted a valid checkout
- **WHEN** the same submission arrives a second time
- **THEN** only one order exists and the second submission returns that same order reference

### Scenario: An undeliverable address stops before payment

- **GIVEN** a shopper whose address is outside the delivery area
- **WHEN** they confirm that address
- **THEN** they are told which destinations are available and are never asked for payment details
