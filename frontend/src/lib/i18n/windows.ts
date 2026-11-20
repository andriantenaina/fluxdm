// Strings of the separate windows (progress, add, video, batch, done).

export const fr = {
  // progress window
  pwDone: 'Terminé', pwCancelYes: 'Annuler le téléchargement', pwKeep: 'Continuer',
  pwNotFound: 'Téléchargement introuvable', pwNotFoundHint: 'Il a peut-être été supprimé de la liste.',
  pwExpand: 'Agrandir la fenêtre', pwMini: 'Mode mini', pwIn: 'en {d}',
  pwSegmentsLabel: 'Carte du fichier : {n} segments', pwLast30: '30 dernières secondes', pwSpeedChart: 'Graphique de vitesse',
  pwActiveConns: 'Connexions actives', pwConnections: 'Connexions', pwSegment: 'Segment {n}',
  pwLimitKB: 'Limite en Ko/s', pwOpenWhenDone: 'Ouvrir le dossier à la fin',
  // add dialog
  addErrScheme: 'Adresse non prise en charge : utilisez http(s)://, ftp:// ou magnet:.',
  addErrInvalid: 'Adresse invalide.', addWebPage: 'Page web', addToQueue: 'Ajouter à la file', addExt: 'Extension',
  addAdvanced: 'Options avancées', addMirrors: 'Miroirs (une adresse par ligne, même fichier)', addMoreOptions: 'Autres options d\'ajout',
  // video dialog
  vidCatVideo: 'Vidéos', vidCatAudio: 'Musique', vidTabVideo: 'Vidéo', vidTabAdvanced: 'Avancé',
  vidErrTitle: 'Analyse impossible', vidErrHint: 'Aucune vidéo n\'a pu être détectée à cette adresse. Réessayez ou téléchargez-la comme un fichier simple.',
  vidAddPlain: 'Ajouter comme fichier simple', vidEst: '≈ {d} à la vitesse actuelle',
  vidMp4: 'MP4 (recommandé)', vidTs: 'TS (flux brut)', vidMp3Desc: 'Compatible partout', vidM4aDesc: 'Meilleure qualité, sans réencodage',
  // batch dialog
  batchEmpty: 'Aucun lien', batchEmptyHint: 'La liste reçue ne contient aucun lien téléchargeable.',
  batchUseCats: 'Ranger par catégorie',
}

export const en: typeof fr = {
  pwDone: 'Done', pwCancelYes: 'Cancel download', pwKeep: 'Keep downloading',
  pwNotFound: 'Download not found', pwNotFoundHint: 'It may have been removed from the list.',
  pwExpand: 'Expand window', pwMini: 'Mini mode', pwIn: 'in {d}',
  pwSegmentsLabel: 'File map: {n} segments', pwLast30: 'Last 30 seconds', pwSpeedChart: 'Speed chart',
  pwActiveConns: 'Active connections', pwConnections: 'Connections', pwSegment: 'Segment {n}',
  pwLimitKB: 'Limit in KB/s', pwOpenWhenDone: 'Open the folder when done',
  addErrScheme: 'Unsupported address: use http(s)://, ftp:// or magnet:.',
  addErrInvalid: 'Invalid address.', addWebPage: 'Web page', addToQueue: 'Add to queue', addExt: 'Extension',
  addAdvanced: 'Advanced options', addMirrors: 'Mirrors (one address per line, same file)', addMoreOptions: 'More add options',
  vidCatVideo: 'Videos', vidCatAudio: 'Music', vidTabVideo: 'Video', vidTabAdvanced: 'Advanced',
  vidErrTitle: 'Analysis failed', vidErrHint: 'No video could be detected at this address. Retry or download it as a plain file.',
  vidAddPlain: 'Add as plain file', vidEst: '≈ {d} at the current speed',
  vidMp4: 'MP4 (recommended)', vidTs: 'TS (raw stream)', vidMp3Desc: 'Plays everywhere', vidM4aDesc: 'Best quality, no re-encoding',
  batchEmpty: 'No links', batchEmptyHint: 'The received list contains no downloadable link.',
  batchUseCats: 'Sort into categories',
}
