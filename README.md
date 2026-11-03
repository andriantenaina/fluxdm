# FluxDM

Gestionnaire de téléchargements libre (Windows 10/11, macOS 12+) avec extension pour Opera, Chrome, Edge et Brave.

## Fonctionnalités

- **Moteur multi-connexion** (1 à 32 connexions) à segmentation dynamique : une connexion libre reprend la moitié du plus gros morceau restant. Il réduit tout seul ses connexions si le serveur les limite.
- **Reprise** après pause, coupure réseau, plantage ou redémarrage.
- **Protocoles** : HTTP/HTTPS (HTTP/2 en option), FTP/FTPS, BitTorrent et liens magnet.
- **Vidéo** :
  - flux HLS (`.m3u8`, segments `.ts`/fMP4, chiffrement AES-128, segments déguisés en images) ;
  - flux DASH (`.mpd`) ;
  - YouTube et environ 1000 sites via yt-dlp ;
  - hébergeurs : Voe, Doodstream, Uqload, Vidmoly, Filemoon, StreamWish, Mixdrop, Streamtape, plus un extracteur générique.
- **Sortie vidéo** en `.ts` brut, ou en MP4/MKV sans réencodage (ffmpeg).
- **Extension navigateur** :
  - capture des téléchargements ;
  - détection des vidéos dans le trafic réseau, y compris dans les lecteurs intégrés en iframe ;
  - bouton « Télécharger la vidéo » sur les lecteurs ;
  - menus clic droit, dont « Télécharger tous les liens ».
- **Organisation** : catégories automatiques, files d'attente, planificateur (démarrage/arrêt, veille ou extinction à la fin), quota de téléchargement, limiteur de vitesse global et par téléchargement.
- **Autres** : aspirateur de site (filtres, profondeur, pages hors ligne, relance périodique), synchronisation de fichiers, analyse antivirus, proxy HTTP/SOCKS5, identifiants par site (Basic/NTLM), surveillance du presse-papier, import/export de liste, thème clair/sombre, interface en français et en anglais.

Limite volontaire : les contenus protégés par DRM (Netflix, Disney+, etc.) ne sont pas pris en charge. Vous êtes responsable du respect des droits d'auteur.

## Structure

| Dossier | Contenu |
|---|---|
| `cmd/fluxdm` | Application de bureau (Go + Wails v3) |
| `cmd/fluxdm-host` | Hôte de liaison native pour l'extension |
| `cmd/fluxdm-cli` | Téléchargeur en ligne de commande (tests, banc d'essai) |
| `internal/engine` | Moteur HTTP multi-connexion |
| `internal/media` | HLS, DASH, yt-dlp, ffmpeg, hébergeurs |
| `internal/extractors` | Recettes d'hébergeurs (`hosters.json`, modifiables sans recompiler) |
| `internal/core`, `internal/store` | Gestionnaire, files d'attente, réglages, SQLite |
| `internal/torrent`, `internal/ftpdl`, `internal/grabber`, `internal/scheduler` | Torrent, FTP, aspirateur de site, planificateur |
| `internal/bridge` | Serveur local appelé par l'extension |
| `internal/sidecars` | Installation à la demande de yt-dlp, ffmpeg et Deno |
| `frontend/` | Interface (Svelte 5 + Vite) |
| `extension/` | Extension MV3 (JavaScript simple, sans compilation) |

## Construire sous Windows

Prérequis : Go 1.26 et Node.js 20 ou plus.

```powershell
cd frontend; npm install; npm run build; cd ..
go build -ldflags "-H windowsgui -s -w" -o build\bin\fluxdm.exe ./cmd/fluxdm
go build -ldflags "-s -w" -o build\bin\fluxdm-host.exe ./cmd/fluxdm-host
```

Gardez `fluxdm-host.exe` dans le même dossier que `fluxdm.exe`. Au démarrage, FluxDM enregistre lui-même l'hôte de liaison native et le protocole `fluxdm://` pour l'utilisateur courant, sans droits administrateur.

Pour la version macOS (binaire universel, `.dmg`), l'installeur Windows (NSIS) et le `.zip` de l'extension, poussez sur GitHub : le workflow `.github/workflows/build.yml` construit tout. Un tag `v*` publie aussi une release.

Pour garder la compatibilité avec macOS 12, restez sur Go 1.26 : les versions suivantes de Go pourraient exiger macOS 13.

## Installer l'extension dans Opera (mode développeur)

1. Ouvrez `opera://extensions` et activez le **Mode développeur**.
2. Cliquez sur **Charger l'extension non empaquetée** et choisissez le dossier `extension/`.
3. Lancez FluxDM une fois. Le badge de l'extension doit afficher « Connecté ».

Grâce à la clé fixée dans `manifest.json`, l'identifiant de l'extension est toujours `lnbmofjmdcfihhamjdgjdhbclmhpfblk`. Si vous publiez sur un store, ajoutez l'identifiant attribué dans `bridge.ExtensionIDs`. Ne publiez jamais `extension/keys/`.

## Tests

```powershell
go test ./internal/...                                          # tests unitaires et de bout en bout locaux
$env:FLUXDM_NET='1'; go test ./internal/media -run RealHLS -v   # vrai flux HLS (réseau)
```
