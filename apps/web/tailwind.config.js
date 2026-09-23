/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: "#1d2a20",
        paper: "#f7f5ed",
        cream: "#eeebdc",
        moss: "#294d37",
        olive: "#778263",
        sky: "#c8e1ee",
        lavender: "#ddd7eb",
        butter: "#f3dea0"
      },
      boxShadow: {
        lift: "0 18px 50px rgba(35, 53, 39, 0.10)"
      },
      animation: {
        "soft-in": "soft-in 300ms ease-out both",
        "soft-pulse": "soft-pulse 1.8s ease-in-out infinite"
      },
      keyframes: {
        "soft-in": { from: { opacity: "0", transform: "translateY(8px)" }, to: { opacity: "1", transform: "translateY(0)" } },
        "soft-pulse": { "0%,100%": { opacity: "0.45" }, "50%": { opacity: "1" } }
      }
    }
  },
  plugins: []
};
