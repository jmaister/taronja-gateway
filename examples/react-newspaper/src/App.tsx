import { Routes, Route, useParams } from 'react-router-dom';
import NewspaperLayout from './components/NewspaperLayout';
import HomePage from './components/HomePage';
import ArticlePage from './components/ArticlePage';
import PremiumArticlePage from './components/PremiumArticlePage';
// LoginPage is removed
import ProtectedRoute from './components/ProtectedRoute';

// Helper component to extract ID for ArticlePage
const ArticlePageWrapper = () => {
  const { id } = useParams<{ id: string }>();
  return <ArticlePage id={id || 'Unknown'} />;
};

// Helper component to extract ID for PremiumArticlePage
const PremiumArticlePageWrapper = () => {
  const { id } = useParams<{ id: string }>();
  return <PremiumArticlePage id={id || 'Unknown'} />;
};

function App() {
  return (
    <Routes>
      {/* LoginPage route removed */}

      {/* Routes with NewspaperLayout */}
      <Route path="/*" element={
        <NewspaperLayout>
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/article/:id" element={<ArticlePageWrapper />} />
            <Route
              path="/premium/article/:id"
              element={
                <ProtectedRoute>
                  <PremiumArticlePageWrapper />
                </ProtectedRoute>
              }
            />
            {/* Add a 404 or default route within the layout if needed */}
            <Route path="*" element={<div>Page not found inside layout</div>} />
          </Routes>
        </NewspaperLayout>
      } />
    </Routes>
  );
}

export default App;
