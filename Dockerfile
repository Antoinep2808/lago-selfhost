FROM getlago/api:v1.33.2

# Expose the Lago API port
EXPOSE 3000

# Force production mode
ENV LAGO_ENV=production
ENV RAILS_ENV=production
ENV RACK_ENV=production

# Start Lago API explicitly in production
CMD ["bash", "-lc", "RAILS_ENV=production bundle exec puma -C config/puma.rb"]