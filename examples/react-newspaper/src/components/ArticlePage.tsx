import React from 'react';

interface ArticlePageProps {
  id: string | number;
}

const ArticlePage: React.FC<ArticlePageProps> = ({ id }) => {
  // Mock titles based on ID
  const titles: { [key: string]: string } = {
    "1": "The Future of Artificial Intelligence",
    "2": "Understanding Global Economic Trends for 2024",
  };
  const articleTitle = titles[String(id)] || `Public Article ${id}`;

  return (
    <article className="bg-white shadow-lg rounded-lg p-6 md:p-8 max-w-3xl mx-auto">
      <header className="mb-6">
        <h1 className="text-3xl md:text-4xl font-bold text-gray-800 font-serif mb-2">
          {articleTitle}
        </h1>
        <p className="text-sm text-gray-500">Published on: {new Date().toLocaleDateString()}</p>
      </header>

      <div className="prose prose-lg max-w-none text-justify text-gray-700">
        <p>
          Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
          Nibh mauris cursus mattis molestie a iaculis at. Magna fermentum iaculis eu non diam phasellus vestibulum lorem sed.
          Amet justo donec enim diam vulputate ut pharetra. Pellentesque habitant morbi tristique senectus et netus et.
          Arcu cursus euismod quis viverra nibh cras pulvinar mattis nunc.
        </p>
        <p>
          Dictumst quisque sagittis purus sit amet volutpat consequat mauris. Eget nullam non nisi est sit amet facilisis magna.
          Risus ultricies tristique nulla aliquet enim tortor at. Augue neque gravida in fermentum et sollicitudin ac orci.
          Nunc sed id semper risus in hendrerit gravida. Praesent semper feugiat nibh sed pulvinar proin gravida hendrerit.
          Congue mauris rhoncus aenean vel elit scelerisque mauris pellentesque. Facilisis leo vel fringilla est ullamcorper eget nulla.
        </p>
        <p>
          Volutpat commodo sed egestas egestas fringilla phasellus faucibus scelerisque. Ut sem viverra aliquet eget sit amet tellus.
          Egestas purus viverra accumsan in nisl nisi scelerisque eu. Enim praesent elementum facilisis leo vel fringilla est.
          Nulla facilisi cras fermentum odio eu feugiat pretium nibh. Id aliquet lectus proin nibh nisl condimentum id.
        </p>
      </div>

      <footer className="mt-8 border-t pt-4">
        <p className="text-sm text-gray-600">Thank you for reading The React Times.</p>
      </footer>
    </article>
  );
};

export default ArticlePage;
