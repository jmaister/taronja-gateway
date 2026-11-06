# Billing: payment cases

Estado: ✅ cubierto (test o prueba en vivo) · ⚠️ parcial · ❌ no cubierto.
Los tests están en `billing/` y `gateway/`; "vivo" = prueba con Stripe de prueba.

| Estado/evento | Acción actual | Problema | Soluciones | Estado |
|---|---|---|---|---|
| Checkout completado, pagado | Crea compra o suscripción y factura | — | — | ✅ TestPayFlow, vivo |
| Checkout completado, no pagado | Queda `awaiting_payment`; aviso en la cuenta | — | — | ✅ TestCheckoutCompletedUnpaid_IsAwaitingPaymentUntilAsyncPaid, TestAccountShowsCheckoutAwaitingPayment (vivo no: ver pago pendiente) |
| Async pagado | Igual que completado | — | — | ✅ TestCheckoutAsyncPayment |
| Pago pendiente (SEPA, Bancontact) en vivo | Aviso "pendiente de confirmación" | En modo test, el checkout de la cuenta solo ofrece tarjeta, iDEAL, Link y PayPal (SEPA está en otra configuración de métodos); el formulario SEPA no se pudo enviar desde el navegador automático | Probar a mano con SEPA en test | ⚠️ solo test unitario |
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
| `incomplete` con prueba | Acceso hasta `trial_until`; aviso | — | — | ✅ TestAccountPaymentProblems, TestStartNow_FailedChargeKeepsAccessUntilOurTrialEnd |
| `incomplete` sin prueba | Sin acceso; aviso "Suscripción no activa (incomplete)" | — | — | ✅ TestAccountPaymentProblems |
| `unpaid` | Cancelada local; etiqueta de Stripe; aviso | En esta cuenta Stripe nunca llega a `unpaid`: tras los reintentos pasa a `canceled` (vivo) | Activar en Stripe "marcar como unpaid" si se quiere | ⚠️ aviso: solo test unitario |
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
