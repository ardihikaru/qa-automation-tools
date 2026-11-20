import { Routes, Route, Navigate } from 'react-router-dom';
import Layout from './components/Layout';
import CostAnalytics from './views/CostAnalytics';
import SkillsView from './views/SkillsView';
import AgentsView from './views/assistants/AgentsView';
import RulesView from './views/assistants/RulesView';
import AssistantDetailView from './views/assistants/AssistantDetailView';
import NavigationGraphView from './views/NavigationGraphView';
import LearningProgressView from './views/LearningProgressView';
import ProductsView from './views/ProductsView';
import KnowledgeView from './views/knowledge/KnowledgeView';
import KnowledgeEntityPage from './views/knowledge/pages/KnowledgeEntityPage';
import JobView from './views/JobView';
import HistoryView from './views/history/HistoryView';
import ConversationView from './views/conversations/ConversationView';
import ArchiveView from './views/conversations/ArchiveView';
import SettingsView from './views/SettingsView';
import LoginView from './views/LoginView';
import ChangePasswordView from './views/ChangePasswordView';
import { ProtectedRoute } from './auth/ProtectedRoute';

function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginView />} />
      <Route
        path="/change-password"
        element={
          <ProtectedRoute>
            <ChangePasswordView />
          </ProtectedRoute>
        }
      />
      <Route
        path="/*"
        element={
          <ProtectedRoute>
            <Layout>
              <Routes>
                <Route path="/" element={<Navigate to="/products" replace />} />
                <Route path="/cost" element={<CostAnalytics />} />
                <Route path="/skills" element={<SkillsView />} />
                <Route path="/agents" element={<AgentsView />} />
                <Route path="/rules" element={<RulesView />} />
                <Route path="/assistants/:productId/:code" element={<AssistantDetailView />} />
                <Route path="/navigation" element={<NavigationGraphView />} />
                <Route path="/learning" element={<LearningProgressView />} />
                <Route path="/products" element={<ProductsView />} />
                <Route path="/products/:id/knowledge" element={<KnowledgeView />} />
                <Route path="/products/:id/knowledge/:type/:code" element={<KnowledgeEntityPage />} />
                <Route path="/jobs/:id" element={<JobView />} />
                <Route path="/history" element={<HistoryView />} />
                <Route path="/qa" element={<Navigate to="/history" replace />} />
                <Route path="/conversations/:id" element={<ConversationView />} />
                <Route path="/archive" element={<ArchiveView />} />
                <Route path="/settings" element={<SettingsView />} />
              </Routes>
            </Layout>
          </ProtectedRoute>
        }
      />
    </Routes>
  );
}

export default App;
