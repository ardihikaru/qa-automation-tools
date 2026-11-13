// /rules: rules, commands and instructions files (CLAUDE.md, AGENTS.md) synced from each
// product's automation repo. `?kind=` picks one kind; empty shows all three.
import AssistantList from './AssistantList';
import { useProductParam, useProducts } from './api';
import { ProductSelect } from './ui';

export default function RulesView() {
  const { products, error } = useProducts();
  const product = useProductParam();
  const productName = products.find((p) => p.id === product.id)?.name;
  return (
    <div className="space-y-6 min-w-0">
      <header>
        <h2 className="text-2xl font-bold text-gray-900">Rules</h2>
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
