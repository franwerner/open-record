## Purpose

Keeps an order's state in step with what the payment provider actually did, including the cases where
the provider changes its mind after the shopper has left.

## Trigger

The payment provider sends a webhook reporting that an authorisation was captured, failed, refunded or
disputed.

## Main flow

1. The system verifies the signature on the delivery and refuses anything it cannot attribute to the
   provider.
2. The system finds the order the event refers to and applies the state change the event describes.
3. The system acknowledges the delivery, so the provider stops retrying it.

## Edge cases

- The same event arrives twice, which it will: the second delivery changes nothing and is still
  acknowledged, so the provider does not keep retrying an event that was already handled.
- Events arrive out of order — a refund before the capture it refunds: the later state wins only if it
  is reachable from the current one, and an unreachable transition is recorded and left for a person
  rather than forced.
- The event names an order that does not exist: the delivery is acknowledged, because retrying will
  never help, and the event is recorded for someone to look at.
- The signature does not verify: the delivery is refused and not acknowledged, and nothing is recorded
  against any order.

## Scenarios

### Scenario: A capture moves the order to paid

- **GIVEN** an order awaiting payment
- **WHEN** a signed capture event for it arrives
- **THEN** the order is paid and the delivery is acknowledged

### Scenario: A repeated delivery is harmless

- **GIVEN** an order already moved to paid by a capture event
- **WHEN** the identical event is delivered again
- **THEN** the order is still paid, nothing else changed, and the delivery is acknowledged

### Scenario: An unsigned delivery is refused

- **GIVEN** any order
- **WHEN** an event arrives whose signature does not verify
- **THEN** no order changes and the delivery is not acknowledged

### Scenario: An event for an unknown order is not retried forever

- **GIVEN** a signed event naming an order reference that does not exist
- **WHEN** it is delivered
- **THEN** it is acknowledged and recorded for review
