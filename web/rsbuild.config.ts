import { defineConfig } from '@rsbuild/core';
import { pluginBabel } from '@rsbuild/plugin-babel';
import { pluginReact } from '@rsbuild/plugin-react';

// 文档：https://rsbuild.rs/config/
export default defineConfig({
  html: {
    title: 'Mx Mail Api',
  },
  server: {
    proxy: {
      // 本地开发时前端仍从相同路径调用 /api，避免把后端地址写进业务代码。
      '/api': 'http://localhost:8080',
      // 公开文档由 Gin 读取根目录 API.md 输出，前端开发环境也必须走后端，避免维护 public 副本。
      '/docs/api.md': 'http://localhost:8080',
    },
  },
  plugins: [
    pluginReact(),
    pluginBabel({
      include: /\.[jt]sx?$/,
      exclude: [/[\\/]node_modules[\\/]/],
      babelLoaderOptions(opts) {
        opts.plugins?.unshift('babel-plugin-react-compiler');
      },
    }),
  ],
});
