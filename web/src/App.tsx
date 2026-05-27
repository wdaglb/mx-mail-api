import { Route, Routes } from 'react-router-dom';
import './App.css';
import { HomePage } from './pages';
import { PublicApiDocument } from './pages/PublicApiDocument';

/**
 * App 只负责顶层路由装配，业务页面逻辑下沉到 pages。
 *
 * 参数：无。
 * 返回值：React 应用根组件。
 * 失败条件：路由组件内部负责展示 API 错误。
 */
const App = () => (
  <Routes>
    <Route path="/docs" element={<PublicApiDocument />} />
    <Route path="*" element={<HomePage />} />
  </Routes>
);

export default App;
