import { useState } from 'react';

function App() {
  const [count, setCount] = useState(0);

  return (
    <div className="min-h-screen flex items-center justify-center bg-[#fafaf7] p-8">
      <div className="text-center">
        <h1 className="text-3xl font-semibold mb-2">Hello, artifact.land</h1>
        <p className="opacity-70 mb-6">Edit index.jsx, then run <code>aland push</code>.</p>
        <button
          onClick={() => setCount(count + 1)}
          className="px-4 py-2 rounded-lg border border-black/10 bg-white hover:bg-black/5 transition"
        >
          Clicked {count} time{count === 1 ? '' : 's'}
        </button>
      </div>
    </div>
  );
}

export default App;
