import React from 'react';

interface PremiumArticlePageProps {
  id: string | number;
}

const PremiumArticlePage: React.FC<PremiumArticlePageProps> = ({ id }) => {
  // Mock titles based on ID
  const titles: { [key: string]: string } = {
    "3": "Deep Dive into Quantum Computing",
    "4": "The Geopolitics of Renewable Energy",
  };
  const articleTitle = titles[String(id)] || `Premium Story ${id}`;

  return (
    <article className="bg-gradient-to-br from-yellow-50 via-amber-50 to-yellow-100 shadow-xl rounded-lg p-6 md:p-8 max-w-3xl mx-auto border-l-4 border-amber-500">
      <header className="mb-6">
        <div className="bg-amber-500 text-white text-xs font-semibold inline-block py-1 px-3 rounded-full mb-3 uppercase">
          Exclusive Content
        </div>
        <h1 className="text-3xl md:text-4xl font-bold text-amber-800 font-serif mb-2">
          {articleTitle}
        </h1>
        <p className="text-sm text-amber-600">Published exclusively for subscribers on: {new Date().toLocaleDateString()}</p>
      </header>
      
      <div className="prose prose-lg max-w-none text-justify text-gray-700">
        <p>
          <em>This is exclusive content for our subscribers. Thank you for supporting The React Times.</em>
        </p>
        <p>
          Vestibulum sed arcu non odio euismod lacinia. At tempor commodo ullamcorper a lacus vestibulum sed arcu. 
          Tellus cras adipiscing enim eu turpis egestas pretium. Pharetra magna ac placerat vestibulum lectus mauris ultrices eros. 
          Nulla pharetra diam sit amet nisl suscipit adipiscing bibendum. Erat nam at lectus urna duis convallis convallis tellus.
        </p>
        <p>
          Quis lectus nulla at volutpat diam ut venenatis tellus in. Feugiat vivamus at augue eget arcu dictum varius duis. 
          Libero enim sed faucibus turpis in eu. Ultricies mi quis hendrerit dolor magna eget est lorem. 
          Id diam vel quam elementum pulvinar etiam non quam lacus. Non curabitur gravida arcu ac tortor dignissim.
        </p>
        <p>
          Adipiscing elit ut aliquam purus sit amet luctus venenatis. Elementum sagittis vitae et leo duis ut diam. 
          Gravida dictum fusce ut placerat orci nulla pellentesque. Ac tortor dignissim convallis aenean et tortor at risus. 
          Purus semper eget duis at tellus at urna condimentum.
        </p>
      </div>
      
      <footer className="mt-8 border-t border-amber-300 pt-4">
        <button className="bg-amber-600 hover:bg-amber-700 text-white font-bold py-2 px-4 rounded focus:outline-none focus:shadow-outline transition-colors duration-150">
          Explore More Premium Content
        </button>
      </footer>
    </article>
  );
};

export default PremiumArticlePage;
