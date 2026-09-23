/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: { ink: "#1d2a20", paper: "#f2f0e8", cream: "#e9e6da", moss: "#294d37", olive: "#778263", sky: "#c8e1ee", lavender: "#ddd7eb", butter: "#f3dea0" },
      boxShadow: { lift: "0 18px 50px rgba(35, 53, 39, 0.10)" },
      animation: { "soft-in": "soft-in 240ms ease-out both" },
      keyframes: { "soft-in": { from: { opacity: "0", transform: "translateY(6px)" }, to: { opacity: "1", transform: "translateY(0)" } } }
    }
  },
  plugins: []
};
