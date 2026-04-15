import { useState, useEffect } from 'react';

interface Doc {
  id: number;
  title: string;
  role: string;
  description: string;
  created_at: string;
}

export default function DocsView() {
  const [docs, setDocs] = useState<Doc[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');

  useEffect(() => {
    fetchDocs();
  }, []);

  const fetchDocs = async () => {
    try {
      const response = await fetch('/api/docs');
      const data = await response.json();
      setDocs(data.docs || []);
    } catch (error) {
      console.error('Failed to fetch docs:', error);
    } finally {
      setLoading(false);
    }
  };

  const filteredDocs = docs.filter(doc => 
    doc.title?.toLowerCase().includes(search.toLowerCase()) ||
    doc.role?.toLowerCase().includes(search.toLowerCase())
  );

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Documentation</h2>
          <p className="text-gray-600">Browse system documentation by role</p>
        </div>
      </div>

      {/* Search */}
      <div className="mb-6">
        <input
          type="text"
          placeholder="Search documentation..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full max-w-md px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
        />
      </div>

      {/* Role filters */}
      <div className="flex gap-2 mb-6">
        {['all', 'admin', 'doctor', 'cashier'].map((role) => (
          <button
            key={role}
            className="px-4 py-2 rounded-lg bg-gray-100 hover:bg-gray-200 text-gray-700 capitalize"
          >
            {role}
          </button>
        ))}
      </div>

      {/* Docs grid */}
      {filteredDocs.length === 0 ? (
        <div className="text-center py-12 text-gray-500">
          <p>No documentation found</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {filteredDocs.map((doc) => (
            <div key={doc.id} className="card hover:shadow-md transition-shadow">
              <div className="card-body">
                <div className="flex items-center gap-2 mb-2">
                  <span className="badge badge-primary capitalize">{doc.role}</span>
                </div>
                <h3 className="text-lg font-semibold mb-2">{doc.title}</h3>
                <p className="text-gray-600 text-sm">{doc.description}</p>
                <button className="mt-4 text-primary-600 hover:text-primary-700 text-sm font-medium">
                  View Documentation →
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}