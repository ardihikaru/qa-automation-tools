import { Routes, Route, Navigate } from 'react-router-dom';
import Layout from './components/Layout';
import DocsView from './views/DocsView';
import QAInsights from './views/QAInsights';
import IssueDebugger from './views/IssueDebugger';
import CostAnalytics from './views/CostAnalytics';
import FixTracking from './views/FixTracking';

function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Navigate to="/docs" replace />} />
        <Route path="/docs" element={<DocsView />} />
        <Route path="/qa" element={<QAInsights />} />
        <Route path="/issues" element={<IssueDebugger />} />
        <Route path="/cost" element={<CostAnalytics />} />
        <Route path="/fixes" element={<FixTracking />} />
      </Routes>
    </Layout>
  );
}

export default App;