// /rules: rules, commands and instructions files (CLAUDE.md, AGENTS.md) synced from each
// product's automation repo. `?kind=` picks one kind; empty shows all three.
import { useSearchParams } from 'react-router-dom';
import { usePageHeader } from '../../components/pageHeader';
import AssistantList from './AssistantList';
import { KIND_META, useProductParam, useProducts } from './api';
import { ProductSelect } from './ui';

export default function RulesView() {
  const { products, error } = useProducts();
  const product = useProductParam();
  const productName = products.find((p) => p.id === product.id)?.name;
  // Commands or instructions picked: title it under a "Rules" crumb ("Rules" alone stays the nav title).
  const [params] = useSearchParams();
  const kind = params.get('kind');
  const one = kind === 'command' || kind === 'instructions' ? KIND_META[kind] : null;
  usePageHeader(one ? { title: one.segment, crumbs: [{ label: 'Rules', to: '/rules' }] } : null);
  return (
    <div className="space-y-6 min-w-0">
      <header>
        <p className="text-gray-600">Rules, slash commands and instructions files from each product&rsquo;s automation repo, refreshed by the product sync.</p>
      </header>
      <AssistantList
        kinds={['rule', 'command', 'instructions']}
        segmented
        one="file"
        plural="files"
        productId={product.id}
        productName={productName}
        toolbar={<ProductSelect value={product.raw} onChange={product.set} products={products} failed={error} />}
      />
    </div>
  );
}
