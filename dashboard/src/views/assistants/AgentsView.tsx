// /agents: the agents (.claude/agents/*.md) synced from each product's automation repo.
import AssistantList from './AssistantList';
import { useProductParam, useProducts } from './api';
import { ProductSelect } from './ui';

export default function AgentsView() {
  const { products, error } = useProducts();
  const product = useProductParam();
  const productName = products.find((p) => p.id === product.id)?.name;
  return (
    <div className="space-y-6 min-w-0">
      <header>
        <h2 className="text-2xl font-bold text-gray-900">Agents</h2>
        <p className="text-gray-600">Sub-agents defined in each product&rsquo;s automation repo, refreshed by the product sync.</p>
      </header>
      <AssistantList
        kinds={['agent']}
        one="agent"
        plural="agents"
        productId={product.id}
        productName={productName}
        toolbar={<ProductSelect value={product.raw} onChange={product.set} products={products} failed={error} />}
      />
    </div>
  );
}
