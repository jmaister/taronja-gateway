# Configures the e2e Odoo database for the gateway's billing — run by
# init.sh through `odoo shell` (env is the superuser environment). Idempotent:
# each run only fixes what is missing, then issues a fresh API key for the
# taronja-billing user and prints "TG_ENV NAME=value" lines for init.sh.

import datetime

# init.sh prepends VERIFACTU = True/False (E2E_ODOO_VERIFACTU).

from odoo import fields

company = env.ref("base.main_company")
spain = env.ref("base.es")

# A Spanish company with the PYMEs chart of accounts.
company.write({
    "name": "Taronja Test SL",
    "vat": "ESB12345674",
    "country_id": spain.id,
    "street": "Carrer de la Pau, 1",
    "city": "Valencia",
    "zip": "46002",
    "email": "facturas@taronja.test",
    "currency_id": env.ref("base.EUR").id,
})
if company.chart_template != "es_pymes":
    env["account.chart.template"].try_loading("es_pymes", company, install_demo=False)
if VERIFACTU:
    # Subject to Veri*Factu, registering in the AEAT's pre-production
    # environment with the test certificate Odoo ships (its NIF is A39200019).
    import base64
    from odoo.tools import misc

    cert_path = "l10n_es_edi_verifactu/demo/certificates/Certificado_RPJ_A39200019_CERTIFICADO_ENTIDAD_PRUEBAS_5_Pre.p12"
    certificate = env["certificate.certificate"].search([("scope", "=", "verifactu")], limit=1)
    if not certificate:
        raw = misc.file_open(cert_path, "rb").read()
        content = base64.encodebytes(raw)
        certificate = env["certificate.certificate"].create({
            "content": content,
            "pkcs12_password": "1234",
            "scope": "verifactu",
        })
    company.write({
        "vat": "ESA39200019",
        "state_id": env.ref("base.state_es_v").id,
        "l10n_es_edi_verifactu_required": True,
        "l10n_es_edi_verifactu_test_environment": True,
        "l10n_es_edi_verifactu_certificate_ids": [(6, 0, certificate.ids)],
    })
env.cr.commit()

journals = env["account.journal"].with_company(company)

# Sales journal: credit notes need their own series (RD 1619/2012).
sales = journals.search([("type", "=", "sale"), ("company_id", "=", company.id)], order="sequence, id", limit=1)
if not sales.refund_sequence:
    sales.refund_sequence = True

# The bank journal Stripe payments are registered in.
stripe_journal = journals.search([("code", "=", "STRP"), ("company_id", "=", company.id)], limit=1)
if not stripe_journal:
    stripe_journal = journals.create({"name": "Stripe", "code": "STRP", "type": "bank", "company_id": company.id})

# Stripe prices include VAT: a copy of the 21 % sales tax, included in price.
taxes = env["account.tax"].with_company(company)
tax = taxes.search([("name", "=", "21% IVA incluido"), ("company_id", "=", company.id)], limit=1)
if not tax:
    base_tax = taxes.search([
        ("company_id", "=", company.id), ("type_tax_use", "=", "sale"), ("amount", "=", 21),
        ("amount_type", "=", "percent"), ("price_include", "=", False),
    ], order="sequence, id", limit=1)
    tax = base_tax.copy({"name": "21% IVA incluido", "price_include_override": "tax_included"})

# Outgoing mail goes to mailpit.
if not env["ir.mail_server"].search([("smtp_host", "=", "mailpit")]):
    env["ir.mail_server"].create({"name": "mailpit", "smtp_host": "mailpit", "smtp_port": 1025, "smtp_encryption": "none"})

env["res.lang"]._activate_lang("es_ES")

# The user the gateway calls Odoo as.
groups = env.ref("account.group_account_invoice") | env.ref("base.group_partner_manager") | env.ref("product.group_product_manager")
user = env["res.users"].with_context(no_reset_password=True).search([("login", "=", "taronja-billing")], limit=1)
if not user:
    user = env["res.users"].with_context(no_reset_password=True).create({
        "name": "Taronja Gateway", "login": "taronja-billing", "email": "billing@taronja.test",
        "company_id": company.id, "company_ids": [(6, 0, [company.id])],
    })
user.write({"group_ids": [(4, g.id) for g in groups], "company_id": company.id, "company_ids": [(4, company.id)]})

env["res.users.apikeys"].search([("user_id", "=", user.id)]).sudo().unlink()
# Odoo 19 requires an expiry on the keys of non-admin users.
expires = fields.Datetime.now() + datetime.timedelta(days=90)
key = env["res.users.apikeys"].with_user(user)._generate("rpc", "taronja-gateway e2e", expires)
env.cr.commit()

for name, value in [
    ("ODOO_API_KEY", key),
    ("ODOO_COMPANY_ID", company.id),
    ("ODOO_SALES_JOURNAL_ID", sales.id),
    ("ODOO_PAYMENT_JOURNAL_ID", stripe_journal.id),
    ("ODOO_TAX_ID", tax.id),
]:
    print(f"TG_ENV {name}={value}")
