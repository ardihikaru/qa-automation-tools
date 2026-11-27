// Display language of the knowledge base (docs/designs/KNOWLEDGE_TRANSLATION.md).
// Module state, set by AuthProvider from the signed-in user's `language`; every knowledge
// read sends it as `?lang=`. UI chrome is always English.
export type KbLang = 'en' | 'id';

let current: KbLang = 'en';
export const setKnowledgeLang = (l: KbLang) => {
  current = l;
};
export const getKnowledgeLang = (): KbLang => current;
