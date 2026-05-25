import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  
  const apiUrl = env.VITE_API_URL || 'http://localhost:8080';
  
  return {
    plugins: [react()],
    server: {
      port: parseInt(env.VITE_DEV_SERVER_PORT || '3000', 10),
      host: true,
      // Reverse-proxied hostnames must be allow-listed in Vite 5.
      // Set VITE_ALLOWED_HOSTS=a,b,c in .env to extend at runtime.
      allowedHosts: [
        'localhost',
        '127.0.0.1',
        'qa-tool.local.mfardiansyah.id',
        ...(env.VITE_ALLOWED_HOSTS || '')
          .split(',')
          .map((h) => h.trim())
          .filter(Boolean),
      ],
      proxy: {
        '/api': {
          target: apiUrl,
          changeOrigin: true,
        },
      },
    },
    build: {
      outDir: 'dist',
      sourcemap: env.VITE_APP_DEBUG === 'true',
    },
  };
});