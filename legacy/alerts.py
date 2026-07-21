import logging
import requests

log = logging.getLogger("QuantumHedge")

class AlertManager:
    def __init__(self, telegram_token=None, telegram_chat_id=None, slack_webhook=None, email_config=None):
        self.telegram_token = telegram_token
        self.telegram_chat_id = telegram_chat_id
        self.slack_webhook = slack_webhook
        self.email_config = email_config

    # ───────────────────────────────
    # TELEGRAM ALERT
    # ───────────────────────────────
    def send_telegram(self, message):
        if not self.telegram_token or not self.telegram_chat_id:
            log.warning("Telegram alert skipped: no config")
            return
        try:
            url = f"https://api.telegram.org/bot{self.telegram_token}/sendMessage"
            data = {"chat_id": self.telegram_chat_id, "text": message}
            requests.post(url, data=data)
            log.info(f"Telegram alert sent: {message}")
        except Exception as e:
            log.error(f"Telegram alert fail: {e}")

    # ───────────────────────────────
    # SLACK ALERT
    # ───────────────────────────────
    def send_slack(self, message):
        if not self.slack_webhook:
            log.warning("Slack alert skipped: no config")
            return
        try:
            requests.post(self.slack_webhook, json={"text": message})
            log.info(f"Slack alert sent: {message}")
        except Exception as e:
            log.error(f"Slack alert fail: {e}")

    # ───────────────────────────────
    # EMAIL ALERT (optionnel)
    # ───────────────────────────────
    def send_email(self, subject, body):
        if not self.email_config:
            log.warning("Email alert skipped: no config")
            return
        try:
            import smtplib
            from email.mime.text import MIMEText
            msg = MIMEText(body)
            msg["Subject"] = subject
            msg["From"] = self.email_config["from"]
            msg["To"] = self.email_config["to"]

            with smtplib.SMTP(self.email_config["smtp"], self.email_config.get("port", 587)) as server:
                server.starttls()
                server.login(self.email_config["user"], self.email_config["password"])
                server.sendmail(self.email_config["from"], [self.email_config["to"]], msg.as_string())
            log.info(f"Email alert sent: {subject}")
        except Exception as e:
            log.error(f"Email alert fail: {e}")

    # ───────────────────────────────
    # GENERIC ALERT
    # ───────────────────────────────
    def send_alert(self, message):
        self.send_telegram(message)
        self.send_slack(message)
        if self.email_config:
            self.send_email("Quantum Hedge Alert", message)
