---
title: Order
description: An order runs from awaiting payment to delivered, and can leave for abandoned, refunded, lost in transit or returned; cancellation is possible until the warehouse picks it.
status: accepted
components: [api, web, cli]
---

## Purpose

Says which states an order can be in and what moves it between them, so that anyone asking "can this
still be cancelled" has one place to look rather than reading every operation that touches an order.

## States and transitions

- awaiting payment → paid (the payment provider confirms the authorisation)
- awaiting payment → abandoned (the shopper does not pay within the checkout window)
- paid → ready to ship (every line is in stock and the warehouse has picked it)
- paid → refunded (the shopper cancels before the warehouse picks it)
- ready to ship → shipped (the carrier accepts the parcel)
- shipped → delivered (the carrier confirms delivery)
- shipped → lost in transit (the carrier reports the parcel missing)
- lost in transit → refunded (the claim is settled in the shopper's favour)
- delivered → returned (the shopper sends it back inside the return window)
- returned → refunded (the warehouse receives and accepts the return)

## Scenarios

### Scenario: A paid order can still be cancelled before picking

- **GIVEN** an order that is paid and has not been picked
- **WHEN** the shopper cancels it
- **THEN** the order is refunded and the warehouse is never asked to pick it

### Scenario: A shipped order cannot be cancelled

- **GIVEN** an order the carrier has accepted
- **WHEN** the shopper asks to cancel it
- **THEN** the order stays shipped and the shopper is told to use the return process instead

### Scenario: An unpaid checkout is abandoned rather than kept

- **GIVEN** an order awaiting payment whose checkout window has passed
- **WHEN** the window closes
- **THEN** the order is abandoned and its stock is released

### Scenario: A lost parcel ends in a refund

- **GIVEN** a shipped order the carrier has reported missing
- **WHEN** the claim is settled in the shopper's favour
- **THEN** the order is refunded

### Scenario: A delivered order can be returned inside the window

- **GIVEN** a delivered order inside its return window
- **WHEN** the warehouse receives and accepts the returned parcel
- **THEN** the order is refunded
