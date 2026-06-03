import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      // Proxy /install.sh -> raw GitHub installer script so users can run
      //   curl -fsSL https://logmind.dev/install.sh | bash
      // without us mirroring the script into the site bundle. The source of
      // truth lives at installer/install.sh in the logmind repo.
      {
        source: "/install.sh",
        destination:
          "https://raw.githubusercontent.com/thrillmade/logmind/main/installer/install.sh",
      },
    ];
  },
};

export default nextConfig;
