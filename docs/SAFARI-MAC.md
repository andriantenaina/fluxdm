# Extension FluxDM pour Safari (Mac, macOS 12 ou plus)

Le fichier `.dmg` de FluxDM contient, en plus de `FluxDM.app`, l'app **FluxDM Safari.app** qui transporte l'extension pour Safari.

Elle n'est pas signée par un compte Apple Developer (payant). Safari accepte quand même les extensions non signées, à condition de l'autoriser.

## Installation

1. Ouvrez le `.dmg` et glissez **FluxDM.app** et **FluxDM Safari.app** dans **Applications**.
2. Lancez **FluxDM Safari.app** une fois.
   - Si macOS refuse de l'ouvrir : clic droit sur l'app → **Ouvrir** → **Ouvrir**.
3. Dans Safari, affichez le menu Développement :
   - macOS 13 ou plus : Safari → **Réglages** → **Avancées** → cochez **Afficher les fonctionnalités pour les développeurs web** ;
   - macOS 12 : Safari → **Préférences** → **Avancées** → cochez **Afficher le menu Développement dans la barre des menus**.
4. Menu **Développement** → cochez **Autoriser les extensions non signées** (mot de passe demandé).
5. Safari → **Réglages/Préférences** → **Extensions** → cochez **FluxDM**, puis autorisez-la sur **tous les sites web**.

> Limite d'Apple sans compte payant : l'option **Autoriser les extensions non signées** se décoche à chaque fois que Safari est quitté. Refaites l'étape 4 après chaque redémarrage de Safari.

## Ce qui change par rapport à Opera / Chrome

Safari ne fournit pas les API de capture des téléchargements ni d'observation du trafic réseau. Sur Safari :

- **fonctionne** : bouton « Télécharger » sur les lecteurs vidéo, popup avec les vidéos détectées dans la page, envoi à FluxDM ;
- **ne fonctionne pas** : capture automatique des téléchargements du navigateur, détection des flux visibles uniquement dans le trafic réseau.

Pour une détection complète, utilisez Opera, Chrome, Edge ou Brave avec l'extension classique.
