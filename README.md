# Toutim Server

Serveur de synchronisation pour l'application [Toutim](https://toutim.app), une application mobile de gestion de collections universelles.

Ecrit en **Go**, sans dépendance CGO, avec **SQLite** embarqué (via `modernc.org/sqlite`).

## Fonctionnalités

- Authentification par email/mot de passe (Argon2id + JWT)
- Synchronisation incrémentale (push/pull avec résolution de conflits LWW)
- Upload/download de fichiers (images)
- Export des données (SQLite, JSON)
- Vérification d'email et réinitialisation de mot de passe (SMTP optionnel)
- Base SQLite isolée par utilisateur
- Auto-hébergeable

## Pré-requis

- **Go 1.26+** (pour le développement / build local)
- **Docker** et **Docker Compose** (pour le déploiement)

---

## Démarrage rapide avec Docker

### 1. Cloner le dépot

```bash
git clone https://github.com/fabienbmusic/toutim-server.git
cd toutim-server
```

### 2. Configurer les variables d'environnement

Créez un fichier `.env` à la racine :

```env
TOUTIM_JWT_SECRET=votre-secret-dau-moins-32-caracteres-ici
```

### 3. Lancer le serveur

```bash
docker compose up -d
```

Le serveur démarre sur `http://localhost:8080`.

Vérifiez qu'il fonctionne :

```bash
curl http://localhost:8080/health
# {"data":{"status":"ok","version":"0.1.0"}}
```

### 4. Arrêter le serveur

```bash
docker compose down
```

Les données sont conservées dans le volume Docker `toutim-data`.

---

## Développement local (sans Docker)

### Build et lancement

```bash
# Build
go build -o server ./cmd/server

# Lancement
TOUTIM_JWT_SECRET="dev-secret-dau-moins-32-caracteres" ./server
```

Ou directement avec `go run` :

```bash
TOUTIM_JWT_SECRET="dev-secret-dau-moins-32-caracteres" go run ./cmd/server
```

### Script de développement

Un script `start-local.sh` est fourni. Il charge les variables depuis un fichier `.env` :

```bash
# Créer le .env
echo 'TOUTIM_JWT_SECRET=dev-secret-dau-moins-32-caracteres' > .env

# Lancer
./start-local.sh
```

---

## Configuration

La configuration suit un ordre de priorité : **Valeurs par défaut** → **`config.toml`** → **Variables d'environnement**.

Les variables d'environnement ont toujours la priorité.

### Variables d'environnement

#### Serveur

| Variable | Description | Défaut |
|----------|-------------|--------|
| `TOUTIM_PORT` | Port d'écoute | `8080` |
| `TOUTIM_HOST` | Adresse d'écoute | `0.0.0.0` |

#### Données

| Variable | Description | Défaut |
|----------|-------------|--------|
| `TOUTIM_DATA_DIR` | Répertoire de stockage (bases de données et fichiers) | `./data` |
| `TOUTIM_MAX_USER_STORAGE` | Taille max de stockage par utilisateur | `500MB` |

#### Authentification

| Variable | Description | Défaut |
|----------|-------------|--------|
| `TOUTIM_JWT_SECRET` | **Obligatoire.** Secret pour la signature JWT (>= 32 caractères) | — |

> Le secret JWT ne doit **jamais** être mis dans `config.toml`. Utilisez uniquement les variables d'environnement.

#### Base de données

| Variable | Description | Défaut |
|----------|-------------|--------|
| `TOUTIM_POOL_SIZE` | Taille du pool de connexions LRU | `100` |

#### Email (SMTP) — optionnel

Si le SMTP n'est pas configuré, les utilisateurs sont automatiquement vérifiés à l'inscription (pas de mail de confirmation).

| Variable | Description | Défaut |
|----------|-------------|--------|
| `TOUTIM_SMTP_HOST` | Serveur SMTP | — |
| `TOUTIM_SMTP_PORT` | Port SMTP | `587` |
| `TOUTIM_SMTP_USERNAME` | Identifiant SMTP | — |
| `TOUTIM_SMTP_PASSWORD` | Mot de passe SMTP | — |
| `TOUTIM_SMTP_FROM_ADDRESS` | Adresse d'expédition | — |
| `TOUTIM_SMTP_FROM_NAME` | Nom d'expédition | `Toutim` |
| `TOUTIM_SMTP_USE_TLS` | TLS implicite (port 465) au lieu de STARTTLS | `false` |
| `TOUTIM_SMTP_BASE_URL` | URL de base pour les liens dans les emails | — |

### Exemple complet avec Docker Compose

```yaml
services:
  toutim:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - toutim-data:/data
    environment:
      - TOUTIM_DATA_DIR=/data
      - TOUTIM_JWT_SECRET=${TOUTIM_JWT_SECRET}
      - TOUTIM_PORT=8080
      # SMTP (optionnel)
      - TOUTIM_SMTP_HOST=smtp.example.com
      - TOUTIM_SMTP_PORT=587
      - TOUTIM_SMTP_USERNAME=noreply@example.com
      - TOUTIM_SMTP_PASSWORD=motdepasse
      - TOUTIM_SMTP_FROM_ADDRESS=noreply@example.com
      - TOUTIM_SMTP_BASE_URL=https://toutim.example.com
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
      interval: 30s
      timeout: 5s
      retries: 3

volumes:
  toutim-data:
```

---

## Architecture

### Stockage des données

```
data/
├── users.db                    # Base globale : comptes, tokens
└── user_data/
    ├── {userId}.db             # Base par utilisateur : collections, items, champs
    └── {userId}_files/         # Fichiers par utilisateur (images)
        ├── img_xxxx.jpg
        └── img_yyyy.png
```

- **SQLite par utilisateur** : chaque utilisateur a sa propre base de données, assurant l'isolation des données
- **Pool de connexions LRU** : les connexions aux bases utilisateurs sont recyclées automatiquement
- **Mode WAL** : activé sur toutes les bases pour de meilleures performances en lecture concurrente

### Sécurité

- Mots de passe hashés avec **Argon2id** (recommandé par l'OWASP)
- **Access tokens JWT** : expiration courte (15 min)
- **Refresh tokens** : rotation à chaque utilisation, stockés sous forme de hash SHA-256
- Vérification d'email requise pour la synchronisation (si SMTP configuré)
- Réinitialisation de mot de passe : invalide toutes les sessions actives
- Conteneur Docker exécuté en tant qu'utilisateur non-root

---

## API

Toutes les routes sont préfixées par `/api`.

### Authentification (public)

| Méthode | Route | Description |
|---------|-------|-------------|
| `POST` | `/api/auth/register` | Inscription |
| `POST` | `/api/auth/login` | Connexion |
| `POST` | `/api/auth/refresh` | Renouvellement de tokens |
| `POST` | `/api/auth/logout` | Déconnexion (révocation du refresh token) |
| `POST` | `/api/auth/verify-email` | Vérification d'email |
| `POST` | `/api/auth/resend-verification` | Renvoi du mail de vérification |
| `POST` | `/api/auth/forgot-password` | Demande de réinitialisation |
| `POST` | `/api/auth/reset-password` | Réinitialisation du mot de passe |
| `DELETE` | `/api/auth/account` | Suppression du compte (authentifié) |

### Synchronisation (authentifié + email vérifié)

| Méthode | Route | Description |
|---------|-------|-------------|
| `POST` | `/api/sync/push` | Envoyer les modifications locales |
| `POST` | `/api/sync/pull` | Récupérer les modifications serveur |
| `GET` | `/api/sync/status` | Statut de synchronisation |

### Fichiers (authentifié)

| Méthode | Route | Description |
|---------|-------|-------------|
| `POST` | `/api/files/upload` | Upload d'image (max 10 MB, JPEG/PNG/WebP/GIF) |
| `GET` | `/api/files/{fileID}` | Téléchargement d'image |
| `DELETE` | `/api/files/{fileID}` | Suppression d'image |

### Export (authentifié)

| Méthode | Route | Description |
|---------|-------|-------------|
| `GET` | `/api/export/sqlite` | Export de la base utilisateur |
| `GET` | `/api/export/json` | Export JSON des collections |

### Santé (public)

| Méthode | Route | Description |
|---------|-------|-------------|
| `GET` | `/health` | Vérification de l'état du serveur |

---

## Déploiement en production

### Recommandations

1. **Reverse proxy** : placez le serveur derrière Nginx ou Caddy pour gérer le TLS (HTTPS)

   **Nginx :**

   ```nginx
   server {
       listen 443 ssl;
       server_name toutim.example.com;

       ssl_certificate /etc/letsencrypt/live/toutim.example.com/fullchain.pem;
       ssl_certificate_key /etc/letsencrypt/live/toutim.example.com/privkey.pem;

       location / {
           proxy_pass http://127.0.0.1:8080;
           proxy_set_header Host $host;
           proxy_set_header X-Real-IP $remote_addr;
           proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
           proxy_set_header X-Forwarded-Proto $scheme;
           client_max_body_size 12M;
       }
   }
   ```

   **Apache** (modules requis : `proxy`, `proxy_http`, `ssl`, `headers`) :

   ```apache
   <VirtualHost *:443>
       ServerName toutim.example.com

       SSLEngine on
       SSLCertificateFile /etc/letsencrypt/live/toutim.example.com/fullchain.pem
       SSLCertificateKeyFile /etc/letsencrypt/live/toutim.example.com/privkey.pem

       ProxyPreserveHost On
       ProxyPass / http://127.0.0.1:8080/
       ProxyPassReverse / http://127.0.0.1:8080/

       RequestHeader set X-Forwarded-Proto "https"
       RequestHeader set X-Real-IP "%{REMOTE_ADDR}s"

       LimitRequestBody 12582912
   </VirtualHost>
   ```

   Pour activer les modules Apache nécessaires :

   ```bash
   sudo a2enmod proxy proxy_http ssl headers
   sudo systemctl reload apache2
   ```

2. **Secret JWT** : utilisez un secret aléatoire robuste (>= 32 caractères)

   ```bash
   openssl rand -base64 48
   ```

3. **Sauvegardes** : sauvegardez régulièrement le répertoire `data/` (ou le volume Docker)

   ```bash
   # Avec Docker
   docker compose exec toutim cp -r /data /data-backup-$(date +%Y%m%d)

   # Ou depuis l'hote
   cp -r /var/lib/docker/volumes/toutim-server_toutim-data/_data ./backup-$(date +%Y%m%d)
   ```

4. **Bind sur localhost** : si derrière un reverse proxy, limitez l'écoute à `127.0.0.1`

   ```env
   TOUTIM_HOST=127.0.0.1
   ```

---

## Structure du projet

```
toutim-server/
├── cmd/server/main.go           # Point d'entrée
├── internal/
│   ├── api/                     # Routes HTTP et middlewares (chi)
│   ├── auth/                    # Authentification, JWT, Argon2id
│   ├── config/                  # Chargement de configuration
│   ├── database/                # Gestion SQLite, pool, migrations
│   │   └── schema/              # Schémas SQL et migrations
│   ├── email/                   # Service d'envoi SMTP
│   ├── export/                  # Export SQLite et JSON
│   ├── files/                   # Upload/download de fichiers
│   ├── model/                   # Modèles de données
│   ├── sync/                    # Moteur de synchronisation
│   └── util/                    # Utilitaires (JSON response, UUID)
├── Dockerfile                   # Build multi-stage Alpine
├── docker-compose.yml
├── start-local.sh               # Script de développement
├── go.mod / go.sum
└── LICENSE                      # GNU AGPL v3.0
```

---

## Licence

[GNU Affero General Public License v3.0](LICENSE)
