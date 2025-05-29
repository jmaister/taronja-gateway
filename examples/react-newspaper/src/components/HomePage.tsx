import React from 'react';
import { Link } from 'react-router-dom';

const HomePage: React.FC = () => {
  return (
    <div className="text-center p-8 md:p-12 bg-white shadow-lg rounded-lg">
      <h1 className="text-4xl md:text-5xl font-bold text-gray-800 mb-6">
        Welcome to The React Times!
      </h1>
      <p className="text-lg text-gray-600 mb-8 max-w-2xl mx-auto">
        Your daily digest of insightful articles, breaking news, and premium content. 
        We are committed to delivering quality journalism in the ever-evolving digital landscape.
        Explore our latest stories or dive into our exclusive premium section.
      </p>
      <div className="space-y-4 sm:space-y-0 sm:space-x-4">
        <h2 className="text-2xl font-semibold text-gray-700 mb-4">Explore Our Public Articles:</h2>
        <ul className="list-none p-0 m-0 flex flex-col sm:flex-row justify-center space-y-4 sm:space-y-0 sm:space-x-6">
          <li>
            <Link 
              to="/article/1" 
              className="text-blue-600 hover:text-blue-800 text-xl font-medium py-2 px-4 border border-blue-600 rounded-md hover:bg-blue-50 transition-colors duration-150"
            >
              Read Article 1: The Future of AI
            </Link>
          </li>
          <li>
            <Link 
              to="/article/2" 
              className="text-blue-600 hover:text-blue-800 text-xl font-medium py-2 px-4 border border-blue-600 rounded-md hover:bg-blue-50 transition-colors duration-150"
            >
              Read Article 2: Global Economic Trends
            </Link>
          </li>
        </ul>
      </div>
    </div>
  );
};

export default HomePage;
