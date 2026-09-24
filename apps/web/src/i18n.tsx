import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export type Locale = "en" | "id";

export type LocationFailure = "unsupported" | "denied" | "timeout" | "unknown";

export type TitlePart = { text: string; accent?: boolean };

export type Messages = {
  documentTitle: string;
  language: string;
  heroEyebrow: string;
  heroTitle: TitlePart[];
  briefAriaLabel: string;
  briefPlaceholder: string;
  mobileBriefPlaceholder: string;
  examples: string[];
  moodTitle: TitlePart[];
  moodBody: string;
  findingPlaces: string;
  foundPlaces: (count: number) => string;
  emptyTitle: TitlePart[];
  emptyBody: string;
  matchBadge: (percent: number) => string;
  aroundBudget: (amount: string) => string;
  evidenceBadge: (percent: number) => string;
  open24Hours: string;
  openUntil: (time: string) => string;
  whyThisFits: string;
  review: string;
  openMaps: string;
  photoCredit: (name: string) => string;
  scoreBreakdown: string;
  evidenceNote: string;
  scoreComponents: Record<string, string>;
  staleList: (error: string) => string;
  refreshFailed: string;
  locationFallback: (message: string) => string;
  location: Record<LocationFailure, string>;
};

const en: Messages = {
  documentTitle: "Where to WFC",
  language: "Language",
  heroEyebrow: "Find your right kind of room",
  heroTitle: [{ text: "Where do you want to " }, { text: "get coffee", accent: true }, { text: " today?" }],
  briefAriaLabel: "Describe your ideal coffee place",
  briefPlaceholder: "e.g. quiet enough for four hours of coding, under 50k, with a musholla…",
  mobileBriefPlaceholder: "Describe your ideal WFC place…",
  examples: [
    "Quiet WFC in Blok M, must have outlets, under 60k",
    "Garden cafe in Kemang for brunch and casual work",
    "Late-night study spot in South Jakarta with Wi-Fi and a prayer room",
  ],
  moodTitle: [{ text: "Pick a " }, { text: "mood.", accent: true }],
  moodBody: "Type your own brief, or start from one of these.",
  findingPlaces: "Finding places…",
  foundPlaces: (count) => `Found: ${count} ${count === 1 ? "place" : "places"}`,
  emptyTitle: [{ text: "No exact room, " }, { text: "yet.", accent: true }],
  emptyBody: "One of your must-haves rules out every verified place. Soften a constraint and the list will return.",
  matchBadge: (percent) => `${percent}% match`,
  aroundBudget: (amount) => `Around ${amount}`,
  evidenceBadge: (percent) => `${percent}% evidence`,
  open24Hours: "Open 24 hours",
  openUntil: (time) => `until ${time}`,
  whyThisFits: "Why this fits",
  review: "Review",
  openMaps: "Open Maps",
  photoCredit: (name) => `Photo: ${name}`,
  scoreBreakdown: "Score breakdown",
  evidenceNote: "Evidence note",
  scoreComponents: { preference_fit: "Preference fit", facilities: "Facilities", distance: "Distance", budget: "Budget", freshness: "Freshness" },
  staleList: (error) => `${error}. Your last good list stays visible.`,
  refreshFailed: "Could not refresh recommendations",
  locationFallback: (message) => `${message}. Showing Jakarta-wide results instead.`,
  location: {
    unsupported: "This browser does not support location-based searches",
    denied: "Location access is needed for “near me” searches. Allow it in your browser, then try again",
    timeout: "Your location request timed out. Try the search again",
    unknown: "Your current location could not be determined",
  },
};

const id: Messages = {
  documentTitle: "Ngopi di Mana?",
  language: "Bahasa",
  heroEyebrow: "Cari tempat ngopi yang pas",
  heroTitle: [{ text: "Ngopi di mana " }, { text: "hari ini", accent: true }, { text: "?" }],
  briefAriaLabel: "Ceritain tempat ngopi ideal kamu",
  briefPlaceholder: "cth. cukup tenang buat ngoding empat jam, di bawah 50k, ada musholla…",
  mobileBriefPlaceholder: "Ceritain tempat WFC ideal kamu…",
  examples: [
    "WFC tenang di Blok M, wajib ada colokan, di bawah 60k",
    "Cafe taman di Kemang buat brunch dan kerja santai",
    "Tempat nugas di Jaksel yang buka malam, ada wifi dan musholla",
  ],
  moodTitle: [{ text: "Cari sesuai " }, { text: "mood.", accent: true }],
  moodBody: "Tulis brief kamu sendiri, atau mulai dari salah satu contoh ini.",
  findingPlaces: "Mencari tempat…",
  foundPlaces: (count) => `Ditemukan: ${count} tempat`,
  emptyTitle: [{ text: "Belum ada tempat yang " }, { text: "pas banget.", accent: true }],
  emptyBody: "Salah satu syarat wajib kamu menyingkirkan semua tempat yang sudah terverifikasi. Longgarkan satu syarat, daftarnya balik lagi.",
  matchBadge: (percent) => `${percent}% cocok`,
  aroundBudget: (amount) => `Sekitar ${amount}`,
  evidenceBadge: (percent) => `${percent}% bukti`,
  open24Hours: "Buka 24 jam",
  openUntil: (time) => `sampai ${time}`,
  whyThisFits: "Kenapa cocok",
  review: "Ulasan",
  openMaps: "Buka Maps",
  photoCredit: (name) => `Foto: ${name}`,
  scoreBreakdown: "Rincian skor",
  evidenceNote: "Catatan bukti",
  scoreComponents: { preference_fit: "Kesesuaian preferensi", facilities: "Fasilitas", distance: "Jarak", budget: "Budget", freshness: "Kebaruan data" },
  staleList: (error) => `${error}. Daftar terakhir kamu tetap tampil.`,
  refreshFailed: "Gagal memperbarui rekomendasi",
  locationFallback: (message) => `${message}. Hasil ditampilkan untuk seluruh Jakarta.`,
  location: {
    unsupported: "Browser ini tidak mendukung pencarian berbasis lokasi",
    denied: "Pencarian “dekat saya” butuh akses lokasi. Izinkan di browser, lalu coba lagi",
    timeout: "Permintaan lokasi kamu kehabisan waktu. Coba cari lagi",
    unknown: "Lokasi kamu sekarang tidak bisa ditentukan",
  },
};

const dictionaries: Record<Locale, Messages> = { en, id };

const STORAGE_KEY = "wfc.locale";

function detectLocale(): Locale {
  const language = (window.localStorage.getItem(STORAGE_KEY) || navigator.language).toLowerCase().split(/[-_]/)[0];
  return language === "id" || language === "in" ? "id" : "en";
}

type I18nValue = { locale: Locale; setLocale: (locale: Locale) => void; messages: Messages };

const I18nContext = createContext<I18nValue | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<Locale>(detectLocale);
  const messages = dictionaries[locale];
  useEffect(() => {
    window.localStorage.setItem(STORAGE_KEY, locale);
    document.documentElement.lang = locale;
    document.title = messages.documentTitle;
  }, [locale, messages]);
  const value = useMemo(() => ({ locale, setLocale, messages }), [locale, messages]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const value = useContext(I18nContext);
  if (!value) throw new Error("useI18n must be used inside I18nProvider");
  return value;
}
