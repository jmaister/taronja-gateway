# Billing: payment cases

Estado: ✅ cubierto (test o prueba en vivo) · ⚠️ parcial · ❌ no cubierto.
Los tests están en `billing/` y `gateway/`; "vivo" = prueba con Stripe de prueba.

| Estado/evento | Acción actual | Problema | Soluciones | Estado |
|---|---|---|---|---|
| Checkout completado, pagado | Crea compra o suscripción y factura | — | — | ✅ TestPayFlow, vivo |
| Checkout completado, no pagado | Queda `awaiting_payment`; aviso en la cuenta | — | — | ✅ TestCheckoutCompletedUnpaid_IsAwaitingPaymentUntilAsyncPaid, TestAccountShowsCheckoutAwaitingPayment (vivo no: ver pago pendiente) |
| Async pagado | Igual que completado | — | — | ✅ TestCheckoutAsyncPayment |
| Pago pendiente (SEPA, Bancontact) | Aviso "pendiente de confirmación"; `awaiting_payment`; al cobrar, compra y factura; al fallar, expirado | En vivo el navegador automático no envía el formulario SEPA | Pagar a mano con IBAN de prueba y reenviar los eventos desde `GET /v1/events` | ✅ e2e `TestBilling_AsyncPaymentPending` (Odoo real, eventos firmados); ⚠️ vivo no |
| Async fallido | Checkout expirado | Sin aviso al cliente | B: email de fallo | ⚠️ sin email |
| Checkout expirado | Cierra el checkout | — | — | ✅ TestParseWebhook_CheckoutExpiredUnpaid |
| Trial, Empezar ya, pago OK | `active`, `trial_until` fijo | — | — | ✅ TestStartNow_*, vivo |
| Trial, Empezar ya, pago fallido | `past_due`; acceso `trial` hasta `trial_until`; aviso | — | — | ✅ TestAccountPaymentProblems, vivo |
| Trial, tarjeta 9995 | Checkout no crea suscripción | Stripe muestra "Se ha rechazado tu tarjeta de crédito debido a que no tiene fondos suficientes. Intenta pagar con una tarjeta de débito." y el cliente sigue en el formulario | Nada útil en el gateway: el mensaje lo pone Stripe en Checkout | ⚠️ vivo: documentado |
| Trial, doble clic | Una clave, un cobro | El segundo clic da error | B: botón deshabilitado | ✅ vivo |
| Trial natural, cobro fallido | `past_due` con gracia | — | — | ✅ vivo, test clock |
| Activación `on_activation` | `awaiting` → `trialing` | — | — | ✅ TestActivateSubscription_FixesOurTrialEnd |
| Activación duplicada | Sin segunda suscripción | — | — | ✅ TestActivateSubscription_DuplicateCallCreatesOneSubscription |
| Activación tras reembolso | Cancela en Stripe | — | — | ✅ TestActivateSubscription_CanceledWhileStripeCreatesIt |
| Renovación OK | `active`, periodo nuevo | — | — | ✅ TestSubscriptionLifecycle |
| Renovación fallida y gracia | `past_due` + aviso | — | — | ✅ TestSubscriptionLifecycle, vivo |
| Fin de gracia | Sin acceso | — | — | ✅ TestCompute_Subscriptions |
| `past_due` | Acceso en gracia; aviso con fecha | — | — | ✅ TestAccountPaymentProblems, vivo |
| `incomplete` con prueba | Acceso `trial` hasta `trial_until` sin gracia; aviso | Vivo: con prueba de 7 días Stripe deja la suscripción en `trialing` aunque el método falle (no hay `incomplete`) | — | ✅ e2e `TestBilling_SubscriptionIncompleteInsideTrial` (evento `incomplete` firmado); ⚠️ vivo no alcanzable |
| `incomplete` sin prueba | Sin acceso; aviso "Suscripción no activa (incomplete)" | — | — | ✅ TestAccountPaymentProblems |
| `unpaid` | Cancelada local; etiqueta de Stripe; aviso; sin acceso | Vivo: con `send_invoice` vencido Stripe deja `past_due` (3, 10 y 45 días); con tarjeta rechazada, tras los reintentos, `canceled`. Nunca `unpaid` en vivo: depende de un ajuste de reintentos de la cuenta, que no tocamos | Activar "marcar como unpaid" en Stripe si se quiere ese estado | ✅ e2e `TestBilling_SubscriptionUnpaidAfterTrial` (evento real firmado); ⚠️ vivo no alcanzable |
| `incomplete_expired` | Cancelada local; etiqueta de Stripe; aviso | — | — | ✅ vivo (test clock), TestAccountPaymentProblems |
| `paused` | Cancelada local; etiqueta de Stripe; aviso | — | — | ✅ vivo (prueba que termina sin método de pago), TestAccountPaymentProblems |
| `past_due` tras reintentos | Stripe la cancela (vivo: `canceled`) | El cliente pierde acceso al agotarse los reintentos | — | ✅ vivo (test clock) |
| Cancelación al final del periodo | `cancel_at_period_end`, acceso hasta el periodo | — | — | ✅ TestCompute_Subscriptions (canceling) |
| Cancelación ahora | `canceled`, sin acceso | — | — | ✅ vivo |
| Reembolso total | Abono; cancela si `awaiting` | — | — | ✅ TestOnActivationPurchaseRefund |
| Reembolso parcial | Abono parcial; clave por compra | — | — | ✅ TestRefundPurchase_DoubleClickRefundsOnce |
| Reembolso desde el dashboard | `charge.refunded` | — | — | ✅ TestRefundFromDashboardCancelsAwaitingActivation |
| Cambio de plan, subir | Cobro prorrateado al momento | — | — | ✅ TestPlanChange_UpgradeThenScheduledDowngrade, vivo |
| Cambio de plan, bajar | Programado al fin del periodo | — | — | ✅ TestPlanChange_UpgradeThenScheduledDowngrade, vivo |
| Cambio de plan, cobro fallido | `past_due`; acceso en gracia; sigue el plan nuevo | — | — | ✅ TestPlanChange_FailedChargeKeepsAccessInGrace |
| Webhook duplicado | `stripe_events` idempotente | — | — | ✅ TestWebhookOrder_DuplicateEvents |
| Webhook fuera de orden | Guarda `stripe_event_at` | — | — | ✅ TestWebhookOrder_EveryOrderEndsActive |
| Webhook perdido | Stripe reintenta; setup avisa a las 2 h | Sin reenvío automático | Reenvío de eventos reales (GET /v1/events firmado, vivo); aviso en setup con pasos | ✅ vivo (resend.js), TestSetupChecks_StuckStripeEvents |
| Odoo caído | Outbox con reintentos | — | — | ✅ TestOutbox_RetriesWithBackoffThenFails |
| Odoo rechaza factura | Estado `failed` en cola | Reintento manual | — | ⚠️ |
| Email caído | Reintenta sin reemitir | — | — | ✅ TestInvoiceEmail_FailureRetriesWithoutReissuing |
| Cambio de tarjeta en el portal | Stripe reintenta; `invoice.paid` y `active` | — | — | ✅ TestPortalCardChange_PaidInvoiceRestoresActive |
| Cliente de Stripe cambiado | Aviso en setup con pasos manuales | No se corrige solo | Revisar cliente en Stripe | ✅ TestCustomerChangedCheck_* (BD con otro `stripe_customer_id`) |
