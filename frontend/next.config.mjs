// The browser only talks to Next.js. Requests to /api/* are proxied to the Go
// server, so there are no CORS problems. Set GO_API_URL if Go runs elsewhere.
const API = process.env.GO_API_URL || "http://localhost:8080";

/** @type {import('next').NextConfig} */
const nextConfig = {
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${API}/api/:path*` }];
  },
};

export default nextConfig;
