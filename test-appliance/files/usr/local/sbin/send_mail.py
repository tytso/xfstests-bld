#!/usr/bin/python3
"""
Email Submission Tool (SMTP and SendGrid)

This script sends email messages using either the SMTP Submission protocol
(Port 587) with STARTTLS encryption or the SendGrid API. It is designed to
be a flexible replacement for the original send-mail.py.

Configuration is read from a config.ini file. If a SendGrid API key is
found in the configuration, the script uses SendGrid for delivery.
Otherwise, it falls back to SMTP using the provided credentials.
"""
import os
import sys
import argparse
import configparser
import smtplib
import ssl
from email.message import EmailMessage

try:
    import sendgrid
    from sendgrid.helpers.mail import Email, Content, Mail as SGMail
    HAS_SENDGRID = True
except ImportError:
    HAS_SENDGRID = False

def main(argv):
    """
    Parse command-line arguments and send an email via SendGrid or SMTP.

    Args:
        argv (list): Command-line arguments.

    The function performs the following steps:
    1. Parses CLI arguments (sender, file, subject, destination, config path).
    2. Loads configuration from config.ini.
    3. Checks for a 'sendgrid_api_key' in the [sendgrid] or [smtp] section.
    4. If an API key is present and the sendgrid library is available:
       - Sends the email using SendGrid's API.
    5. Otherwise:
       - Establishes an SMTP connection on port 587.
       - Handles TLS based on the 'tls_mode' configuration.
       - Authenticates and sends the message via SMTP.
    """
    parser = argparse.ArgumentParser(description='Send mail using SMTP or SendGrid.')
    parser.add_argument('--sender', help='from address')
    parser.add_argument('-f', '--file', help='input file')
    parser.add_argument('-s', '--subject', help='subject line',
                        default='Report')
    parser.add_argument('dest', help='Destination address')
    parser.add_argument('--config', help='Path to config.ini file', default='config.ini')
    args = parser.parse_args()

    # Read configuration
    config = configparser.ConfigParser()
    config_paths = [
        args.config,
        os.path.expanduser('~/.send-mail.ini'),
        '/etc/send-mail.ini',
        '/run/send-mail.ini',
        '/run/send_mail.ini',
    ]
    config_found = False
    for path in config_paths:
        if os.path.exists(path):
            config.read(path)
            config_found = True
            break

    # Check for SendGrid configuration
    sendgrid_api_key = os.environ.get('SENDGRID_API_KEY')
    if config_found:
        if not sendgrid_api_key:
            sendgrid_api_key = config.get('sendgrid', 'api_key', fallback=None)
        if not sendgrid_api_key:
            # Check fallback section for convenience
            sendgrid_api_key = config.get('smtp', 'sendgrid_api_key', fallback=None)

    receivers = [r.strip() for r in args.dest.split(',')]

    # Prepare email content
    if args.file is None:
        content_text = sys.stdin.read()
    else:
        with open(args.file, 'r') as f:
            content_text = f.read()

    # Determine sender
    if args.sender:
        from_email_addr = args.sender
    elif config_found:
        from_email_addr = config.get('smtp', 'sender', fallback=receivers[0])
    else:
        from_email_addr = receivers[0]

    # Delivery Logic
    if sendgrid_api_key:
        if not HAS_SENDGRID:
            print("Error: SendGrid API key found but 'sendgrid' library is not installed.")
            sys.exit(1)

        try:
            sg = sendgrid.SendGridAPIClient(api_key=sendgrid_api_key)
            from_email = Email(from_email_addr)
            content = Content("text/plain", content_text)
            # SendGrid Mail object takes multiple receivers slightly differently in the helper
            # but the original script passed the list directly.
            mail = SGMail(from_email, receivers, args.subject, content)
            response = sg.client.mail.send.post(request_body=mail.get())
            status = response.status_code
            if status // 100 != 2:
                print(f"SendGrid Error Status: {status}")
                print(response.body)
                sys.exit(1)
            print("Email sent successfully via SendGrid.")
            return
        except Exception as e:
            print(f"Failed to send email via SendGrid: {e}")
            sys.exit(1)

    # Fallback to SMTP

    smtp_server = os.environ.get('SMTP_SERVER')
    smtp_port = os.environ.get('SMTP_PORT')
    smtp_user = os.environ.get('SMTP_USER')
    smtp_password = os.environ.get('SMTP_PASSWORD')
    smtp_cafile = os.environ.get('SMTP_CAFILE')
    tls_mode = os.environ.get('TLS_MODE')

    if config_found:
        try:
            if not smtp_server:
                smtp_server = config.get('smtp', 'server')
            if not smtp_port:
                smtp_port = config.getint('smtp', 'port', fallback=587)
            if not smtp_user:
                smtp_user = config.get('smtp', 'user')
            if not smtp_password:
                smtp_password = config.get('smtp', 'password')
            if not smtp_cafile:
                smtp_cafile = config.get('smtp', 'cafile', fallback=None)
            if not tls_mode:
                tls_mode = config.get('smtp', 'tls_mode',
                                      fallback='mandatory').lower()
        except (configparser.NoSectionError, configparser.NoOptionError) as e:
            print(f"Error in configuration file for SMTP fallback: {e}")
            sys.exit(1)

    if not smtp_port:
        smtp_port = 587
    if not tls_mode:
        tls_mode = 'mandatory'
    if not smtp_server:
        print("Missing SMTP_SERVER")
        sys.exit(1)
    if smtp_user:
        if not smtp_password:
            print("Missing SMTP_PASSWORD")
            sys.exit(1)

    msg = EmailMessage()
    msg.set_content(content_text)
    msg['Subject'] = args.subject
    msg['To'] = ', '.join(receivers)
    msg['From'] = from_email_addr

    smtp_protocol = "SMTP"
    try:
        context = ssl.create_default_context()
        if smtp_cafile:
            context.load_verify_locations(cafile=smtp_cafile)

        smtp_port = int(smtp_port)
        if smtp_port == 465:
            smtp_protocol = "SMTP_SSL"
            with smtplib.SMTP_SSL(smtp_server, smtp_port, context=context) as server:
                if smtp_user:
                    server.login(smtp_user, smtp_password)
                server.send_message(msg)
        else:
            smtp_protocol = "SMTP"
            with smtplib.SMTP(smtp_server, smtp_port) as server:
                if tls_mode == 'disabled':
                    pass
                elif tls_mode == 'optional':
                    try:
                        server.starttls(context=context)
                        smtp_protocol = "SMTP with STARTTLS"
                    except smtplib.SMTPException as e:
                        print(f"Warning: STARTTLS failed in optional mode: {e}")
                else: # mandatory or default
                    server.starttls(context=context)
                    smtp_protocol = "SMTP with STARTTLS"

                if smtp_user:
                    server.login(smtp_user, smtp_password)
                server.send_message(msg)
        print(f"Email sent successfully via {smtp_protocol}.")
    except Exception as e:
        print(f"Failed to send email via {smtp_protocol}: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main(sys.argv)
