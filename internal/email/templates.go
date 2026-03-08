package email

import (
	"bytes"
	"fmt"
	"text/template"
)

type verificationData struct {
	AppName string
	Link    string
}

type passwordResetData struct {
	AppName        string
	Link           string
	ExpirationMins int
}

var verificationTemplate = template.Must(template.New("verify").Parse(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body style="margin:0;padding:0;background-color:#F8F9FA;font-family:'Inter',-apple-system,BlinkMacSystemFont,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#F8F9FA;padding:40px 20px;">
<tr><td align="center">
<table width="100%" style="max-width:480px;background:#FFFFFF;border-radius:16px;border:1px solid #E8E8E8;padding:40px;">
<tr><td style="text-align:center;padding-bottom:24px;">
  <h1 style="font-family:'Quicksand',sans-serif;color:#333333;font-size:24px;font-weight:700;margin:0;">{{.AppName}}</h1>
</td></tr>
<tr><td style="text-align:center;padding-bottom:16px;">
  <h2 style="color:#333333;font-size:20px;font-weight:600;margin:0;">Vérifiez votre adresse email</h2>
</td></tr>
<tr><td style="text-align:center;padding-bottom:32px;">
  <p style="color:#666666;font-size:15px;line-height:1.5;margin:0;">
    Bienvenue sur Toutim ! Cliquez sur le bouton ci-dessous pour confirmer votre adresse email et activer votre compte.
  </p>
</td></tr>
<tr><td style="text-align:center;padding-bottom:32px;">
  <a href="{{.Link}}" style="display:inline-block;background-color:#2D9CDB;color:#FFFFFF;text-decoration:none;font-weight:600;font-size:16px;padding:14px 32px;border-radius:12px;">
    Vérifier mon email
  </a>
</td></tr>
<tr><td style="text-align:center;">
  <p style="color:#999999;font-size:13px;line-height:1.5;margin:0;">
    Si vous n'avez pas créé de compte Toutim, vous pouvez ignorer cet email.
  </p>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`))

var passwordResetTemplate = template.Must(template.New("reset").Parse(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body style="margin:0;padding:0;background-color:#F8F9FA;font-family:'Inter',-apple-system,BlinkMacSystemFont,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#F8F9FA;padding:40px 20px;">
<tr><td align="center">
<table width="100%" style="max-width:480px;background:#FFFFFF;border-radius:16px;border:1px solid #E8E8E8;padding:40px;">
<tr><td style="text-align:center;padding-bottom:24px;">
  <h1 style="font-family:'Quicksand',sans-serif;color:#333333;font-size:24px;font-weight:700;margin:0;">{{.AppName}}</h1>
</td></tr>
<tr><td style="text-align:center;padding-bottom:16px;">
  <h2 style="color:#333333;font-size:20px;font-weight:600;margin:0;">Réinitialisation du mot de passe</h2>
</td></tr>
<tr><td style="text-align:center;padding-bottom:32px;">
  <p style="color:#666666;font-size:15px;line-height:1.5;margin:0;">
    Vous avez demandé la réinitialisation de votre mot de passe. Cliquez sur le bouton ci-dessous pour choisir un nouveau mot de passe. Ce lien expire dans {{.ExpirationMins}} minutes.
  </p>
</td></tr>
<tr><td style="text-align:center;padding-bottom:32px;">
  <a href="{{.Link}}" style="display:inline-block;background-color:#2D9CDB;color:#FFFFFF;text-decoration:none;font-weight:600;font-size:16px;padding:14px 32px;border-radius:12px;">
    Réinitialiser mon mot de passe
  </a>
</td></tr>
<tr><td style="text-align:center;">
  <p style="color:#999999;font-size:13px;line-height:1.5;margin:0;">
    Si vous n'avez pas fait cette demande, vous pouvez ignorer cet email. Votre mot de passe ne sera pas modifié.
  </p>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`))

// RenderVerificationEmail renders the verification email HTML.
func RenderVerificationEmail(baseURL, token string) (subject string, body string, err error) {
	link := fmt.Sprintf("%s/api/auth/verify-email?token=%s", baseURL, token)
	var buf bytes.Buffer
	if err := verificationTemplate.Execute(&buf, verificationData{
		AppName: "Toutim",
		Link:    link,
	}); err != nil {
		return "", "", fmt.Errorf("render verification email: %w", err)
	}
	return "Vérifiez votre adresse email — Toutim", buf.String(), nil
}

// RenderPasswordResetEmail renders the password reset email HTML.
func RenderPasswordResetEmail(baseURL, token string, expirationMins int) (subject string, body string, err error) {
	link := fmt.Sprintf("%s/api/auth/reset-password?token=%s", baseURL, token)
	var buf bytes.Buffer
	if err := passwordResetTemplate.Execute(&buf, passwordResetData{
		AppName:        "Toutim",
		Link:           link,
		ExpirationMins: expirationMins,
	}); err != nil {
		return "", "", fmt.Errorf("render password reset email: %w", err)
	}
	return "Réinitialisation de votre mot de passe — Toutim", buf.String(), nil
}
