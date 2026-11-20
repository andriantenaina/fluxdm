# FluxDM sur iPhone (iOS 27) avec Safari — FluxDM Remote

FluxDM Remote transforme l'iPhone en **télécommande** de FluxDM :

- ajouter des liens et des vidéos trouvées dans Safari ;
- suivre les téléchargements en direct ;
- **récupérer les fichiers terminés** dans l'app Fichiers ou dans Photos.

C'est l'ordinateur (PC Windows ou Mac sous macOS 12 ou plus) qui télécharge. Il n'y a **aucune app à installer** sur l'iPhone : c'est gratuit, sans câble, sans Xcode, et **rien n'expire**.

Prérequis : l'ordinateur est allumé, FluxDM 0.2 ou plus est lancé, et l'iPhone est sur le **même Wi-Fi** (ou relié par Tailscale, voir l'étape 9).

---

## 1. Activer FluxDM Remote sur l'ordinateur

1. Ouvrez FluxDM → **Options** → onglet **iPhone / Réseau**.
2. Activez **FluxDM Remote** (port par défaut : `52780`).
3. Autorisez le pare-feu à la première activation :
   - **Windows** : cochez **Réseaux privés** uniquement, puis **Autoriser l'accès** ;
   - **Mac** : cliquez **Autoriser** quand macOS demande d'accepter les connexions entrantes.

## 2. Appairer l'iPhone

1. Dans le même onglet, cliquez **Générer un QR code**. Le code est valable 10 minutes et ne sert qu'une fois.
2. Sur l'iPhone, ouvrez l'**Appareil photo**, visez le QR code, puis touchez la bannière qui apparaît.
3. Safari ouvre FluxDM Remote : l'iPhone est appairé. Il apparaît dans la liste **Appareils** de l'ordinateur.

## 3. Installer l'icône sur l'écran d'accueil

Restez sur la page FluxDM Remote qui vient de s'ouvrir, **sans changer de page** :

1. Touchez **Partager** (carré avec une flèche).
2. Choisissez **Sur l'écran d'accueil**, puis **Ajouter**.

L'icône FluxDM s'ouvre désormais comme une app, en plein écran, avec quatre onglets : **En cours**, **Ajouter**, **Terminés**, **Réglages**.

> Faites cette étape juste après l'appairage : l'icône garde la connexion de l'iPhone. Si l'icône affiche « non appairé », refaites les étapes 2 et 3.

## 4. Ajouter un lien

Onglet **Ajouter** → **Coller** → **Analyser**. FluxDM affiche le titre, les qualités (`1080p · MP4 · ~1,2 Go`) et le nom du fichier. Choisissez une qualité, puis **Télécharger sur l'ordinateur**.

**Télécharger tel quel** envoie le lien sans analyse (fichier direct, torrent `magnet:`).

## 5. Créer le signet « FluxDM vidéo » (détection des vidéos dans Safari)

1. Dans FluxDM Remote → **Réglages** → **Signet « FluxDM vidéo »** → **Copier le code**.
2. Dans Safari, sur n'importe quelle page : **Partager** → **Ajouter un signet** → nommez-le `FluxDM vidéo` → **Enregistrer**.
3. Ouvrez les **Signets** (icône livre) → **Modifier** → touchez le signet `FluxDM vidéo`.
4. Effacez l'adresse et **collez** le code copié (il commence par `javascript:`) → **OK**.

## 6. Utiliser le signet

1. Ouvrez la page de la vidéo dans Safari et **lancez la lecture** quelques secondes.
2. Touchez **Signets** → **FluxDM vidéo**.
3. La page FluxDM s'ouvre avec ce qui a été trouvé (flux vidéo, lecteur intégré, page entière) et les qualités. Choisissez, puis **Télécharger sur l'ordinateur**.

Si rien n'est trouvé dans le flux, choisissez **Page entière** : l'ordinateur analysera la page lui-même (YouTube et environ 1000 sites, hébergeurs).

## 7. Raccourci « Télécharger avec FluxDM » dans le menu Partager

1. Dans FluxDM Remote → **Réglages** → **Raccourci** → **Copier l'adresse** (`http://<ordinateur>:52780/add?u=`).
2. Ouvrez l'app **Raccourcis** → **+**.
3. **Ajouter une action** → cherchez **Ouvrir les URL**.
4. Dans le champ URL, collez l'adresse copiée, puis, juste après `u=`, insérez la variable **Entrée du raccourci**.
5. Touchez **ⓘ** (détails) → activez **Afficher dans la feuille de partage** → types acceptés : **URL** et **Pages web Safari**.
6. Nommez le raccourci **Télécharger avec FluxDM**.

Utilisation : dans Safari (ou YouTube, etc.) → **Partager** → **Télécharger avec FluxDM**. La page FluxDM s'ouvre avec les qualités.

## 8. Récupérer un fichier sur l'iPhone

1. Icône FluxDM → onglet **Terminés** → **Récupérer** sur le fichier.
2. Safari demande de télécharger : acceptez. Le fichier arrive dans **Fichiers** → **Téléchargements**.
3. Pour mettre une vidéo dans **Photos** : ouvrez-la dans Fichiers → **Partager** → **Enregistrer la vidéo**.

## 9. Hors de la maison (facultatif, gratuit) : Tailscale

1. Installez **Tailscale** sur l'ordinateur (tailscale.com) et sur l'iPhone (App Store), et connectez-vous avec le **même compte**.
2. Dans FluxDM, refaites **Générer un QR code** : un QR « Tailscale » apparaît en plus du QR « Réseau local ».
3. Scannez le QR Tailscale, puis refaites l'étape 3.

Aucun port n'est ouvert sur la box et le trafic est chiffré.

## Cas particulier : iPhone connecté au point d'accès (hotspot) d'un PC

**Le hotspot est sur un autre PC du même réseau** (iPhone → hotspot du PC B → box ← PC A avec FluxDM) :

- rien de spécial à faire : le PC B relaie les demandes de l'iPhone vers le PC A ;
- scannez le QR **Réseau local** affiché par FluxDM sur le PC A (adresse du type `192.168.1.x`) ;
- pour FluxDM, les demandes semblent venir du PC B, ce qui ne change rien à la sécurité : le jeton de l'iPhone reste exigé.

**Le hotspot est sur le PC qui fait tourner FluxDM** (iPhone → hotspot du PC A) :

- FluxDM affiche un QR supplémentaire pour l'adresse du hotspot (souvent `192.168.137.1` sous Windows) : scannez **celui-là** ;
- Windows classe souvent le réseau du hotspot en **réseau public** ; si la page ne s'ouvre pas, autorisez FluxDM aussi sur les réseaux publics :
  - Panneau de configuration → Pare-feu Windows Defender → **Autoriser une application** ;
  - **Modifier les paramètres** → FluxDM → cochez **Public** → OK.

Dans les deux cas, si l'iPhone change de réseau, l'adresse change : refaites les étapes 2 et 3.

## 10. Dépannage

| Problème | Solution |
|---|---|
| Le QR n'ouvre rien ou la page ne charge pas | Vérifiez que l'iPhone est sur le même Wi-Fi. Certaines box isolent les appareils (« isolation client », « AP isolation ») : désactivez-la, ou utilisez Tailscale. Vérifiez l'autorisation du pare-feu (étape 1). |
| « Code d'appairage invalide ou expiré » | Le code dure 10 minutes et ne sert qu'une fois : générez-en un nouveau. |
| « Trop de tentatives » | Attendez une minute. |
| L'app affiche « non appairé » | L'appareil a été révoqué, ou l'icône a été ajoutée depuis une autre page : refaites les étapes 2 et 3. |
| L'adresse de l'ordinateur a changé | Votre box a donné une autre adresse IP à l'ordinateur : refaites les étapes 2 et 3 (ou fixez l'adresse IP dans la box). |
| Le signet ne fait rien | Vérifiez que l'adresse du signet commence par `javascript:` et qu'elle a été collée en entier. |
| Fichier `.ts` illisible sur l'iPhone | FluxDM → Options → Vidéo : choisissez la sortie **MP4**. |
| Récupération lente | Normal : la vitesse est celle du Wi-Fi entre l'iPhone et l'ordinateur. |

**Sécurité** : chaque iPhone reçoit un jeton secret ; seul son empreinte est enregistrée sur l'ordinateur. Pour couper l'accès d'un appareil : Options → iPhone / Réseau → **Révoquer**. Désactivez FluxDM Remote si vous ne l'utilisez pas sur un réseau public.

---

## Annexe : app iOS native (non retenue)

Pour mémoire, une vraie app installée sur l'iPhone n'est possible gratuitement qu'avec un **câble** (Sideloadly, AltStore ou SideStore + Apple ID gratuit) et **expire tous les 7 jours** (3 apps maximum). Un compte Apple Developer (99 $/an) lève ces limites. L'App Store refuse les téléchargeurs YouTube. FluxDM Remote évite toutes ces contraintes.
